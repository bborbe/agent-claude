---
status: completed
spec: [001-cluster-worker-heartbeat]
summary: ActivityRecorder now notifies the publisher of each served session so the publisher stamps it immediately (non-blocking send, outside the mutex), with the 20s ticker unchanged and refreshOne gated on the idle cutoff; tests, README and CHANGELOG updated.
execution_id: agent-claude-cluster-heartbeat-exec-006-spec-001-stamp-immediately-on-record
dark-factory-version: v0.196.0
created: "2026-10-05T20:30:00Z"
queued: "2026-10-05T20:35:46Z"
started: "2026-10-05T20:36:21Z"
completed: "2026-10-05T20:41:28Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. WHY THIS EXISTS. Spec 001's third acceptance criterion requires the reader to return a
   served worker at t=0 of its sampling window. Measured on the live dev cluster 2026-10-05,
   it does not: the sample at t=0 read `[]`, and the first stamp appeared at the next tick.
   With `RefreshInterval = 20s` and a 60 s reader TTL, a session that has just been served
   reads DEAD for up to 20 seconds. That is not cosmetic — a dead reading is what permits a
   second writer onto one conversation, which is the exact defect the cluster store exists to
   prevent. So this is a real gap the criterion caught, not a criterion that needs relaxing.

2. SETTLED — the write must NOT move onto the request path. `observingSession.Prompt` calls
   `recorder.Record` after the delegate returns, and that call sits between the turn finishing
   and the HTTP response being written. Writing the ConfigMap synchronously there would put a
   cluster round-trip in front of every prompt response. The notification therefore travels to
   the publisher, which is already the single writer, and the publisher does the write in its
   own loop.

3. SETTLED — the channel send must never block. `Record` is called on the request path, so a
   full channel must drop the notification rather than wait: the 20 s ticker already covers
   every active session, so a dropped notification delays a stamp by at most one interval
   instead of stalling a prompt.

4. The `ActivityRecorder` interface gains one method. The existing `Record` and `Active`
   signatures are unchanged, so `activity_test.go`'s existing specs keep passing.
-->

# Stamp a served session's heartbeat immediately, not on the next tick

<summary>
- A session that has just been served becomes visible to the cluster liveness reader at once, instead of up to 20 seconds later.
- The 20-second refresh ticker is unchanged and still re-stamps every active session.
- The immediate stamp does not delay the prompt response: the write happens in the publisher's own loop, not on the request path.
- A notification that cannot be delivered is dropped rather than waited on, so a busy writer can never stall a prompt.
- The existing recorder behaviour, the entry shape, the idle cutoff and the reader's contract are all unchanged.
- Unit tests cover the immediate write, the non-blocking send, and that the ticker still refreshes.
</summary>

<objective>
Make a served session's heartbeat entry appear immediately rather than on the next refresh tick. A session id is handed to the single writer the moment it is recorded, and the writer stamps it at once — while still refreshing every active session every `RefreshInterval`. End state: a prompt that completes at t=0 leaves a fresh entry in the ConfigMap before the next tick, so the reader returns the worker at t=0.
</objective>

<context>
This continues spec `001-cluster-worker-heartbeat`. Read the spec at
`specs/in-progress/001-cluster-worker-heartbeat.md` — acceptance criteria 3 and 5 are the
ones this prompt serves — and read the landed package before changing anything. Read
`docs/dod.md` (the Definition of Done you are graded against).

Files you will read and change (repo-relative):
- `pkg/heartbeat/activity.go` — `ActivityRecorder` interface, `NewActivityRecorder`, `activityRecorder.Record` and `.Active`.
- `pkg/heartbeat/publisher.go` — `Publisher` interface, `NewPublisher`, `publisher.Run` and `.refresh`.
- `pkg/heartbeat/activity_test.go`, `pkg/heartbeat/publisher_test.go` — the existing specs, which must keep passing.
- `pkg/heartbeat/observer.go` — the caller of `Record`. **Read it, do not change it.**

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Give `ActivityRecorder` a notification channel.** Add exactly one method to the interface in `pkg/heartbeat/activity.go`:

   ```go
   // Notifications returns the channel on which Record hands out the id of a
   // session that has just been served. A receiver must drain it; a send that
   // cannot proceed is dropped rather than waited on, because Record runs on
   // the prompt request path.
   Notifications() <-chan string
   ```

   `NewActivityRecorder` creates the channel **buffered** (capacity 64) and stores it on the struct. `Record` keeps its current signature and its current timestamp write, and then sends the id.

   The send is **non-blocking** and that is load-bearing, not stylistic: `Record` is called between a turn finishing and the HTTP response being written, so a blocking send would let a busy publisher stall a prompt.

   **Take the send outside the mutex.** `Record` currently holds `a.mutex` under a `defer`; restructure it so the lock covers only the map write and is released before the send — the mutex guards `lastActivity`, and a channel operation has no business inside it:

   ```go
   func (a *activityRecorder) Record(ctx context.Context, sessionID string) {
       a.mutex.Lock()
       a.lastActivity[sessionID] = a.currentDateTime.Now()
       a.mutex.Unlock()

       select {
       case a.notifications <- sessionID:
       default:
           // The ticker covers every active session, so a dropped notification costs
           // at most one refresh interval rather than stalling the caller.
       }
   }
   ```

2. **Make the publisher write a notified session at once — but only while it is still active.** In `pkg/heartbeat/publisher.go`, add a third case to `Run`'s select:

   ```go
   case sessionID := <-p.recorder.Notifications():
       p.refreshOne(ctx, sessionID)
   ```

   Add `refreshOne`, and extract the per-session write into a `writeOne` so the failure handling (`glog.Warningf` and continue) has exactly one implementation:

   ```go
   // refreshOne stamps one notified session, but only while it is still active.
   //
   // The cutoff is checked here and not only in refresh because a notification
   // can be drained late — the loop may have been inside a slow write — and a
   // session that has since gone idle must not be stamped back to life.
   func (p *publisher) refreshOne(ctx context.Context, sessionID string) {
       if !slices.Contains(p.recorder.Active(ctx, IdleCutoff), sessionID) {
           return
       }
       p.writeOne(ctx, sessionID)
   }

   // writeOne stamps one session. A failing write is logged and swallowed: a
   // broken liveness path must not take prompt serving down.
   func (p *publisher) writeOne(ctx context.Context, sessionID string) {
       if err := p.writer.Write(ctx, sessionID); err != nil {
           glog.Warningf("cluster heartbeat write failed session=%s: %v", sessionID, err)
       }
   }
   ```

   `refresh` keeps its `ctx.Done()` check between sessions and now calls `p.writeOne(ctx, sessionID)`. Import `slices`.

   **The activity check in `refreshOne` is load-bearing, not defensive padding.** Without it the notification path stamps unconditionally, which makes an idle session look live whenever a notification is drained after the session went quiet — and it breaks the two existing "idle past the cutoff" specs, whose premise is exactly that an idle session is never written.

   **Do not remove the ticker.** The ticker is what keeps a session live across the TTL after its last prompt; the notification only makes the *first* stamp immediate.

3. **Do not change `Record`'s signature, `Active`, the entry shape, `IdleCutoff`, `RefreshInterval`, `ConfigMapName`, `writer.go`, or `observer.go`.** `observer.go` already calls `recorder.Record(ctx, s.id)` after the delegate returns and needs no change — the immediate write now happens because `Record` notifies, not because the observer does anything new.

4. **Test the immediate write, the non-blocking send, and the unchanged ticker.**

   a. In `pkg/heartbeat/publisher_test.go`, add a spec that proves a notified session is written **without waiting for a tick**: construct a publisher with a long interval (e.g. `time.Hour`) so no tick can fire, start the loop with the existing `start()` helper, call `recorder.Record(ctx, "abc")`, then `Eventually(deployer.DeployCallCount).Should(Equal(1))` and assert the deployed object carries key `"abc"`. **Pass the method value `deployer.DeployCallCount`, never a call `deployer.DeployCallCount()`** — Gomega's poller returns a constant when the actual is not a func, so a called method snapshots the count once and never re-polls, and the assertion would pass or fail on timing rather than on behaviour. The existing file already uses the method-value form; match it. A publisher that only writes on the ticker fails this spec — that is the point of it.

   b. Add a spec proving the send never blocks: fill the channel past its capacity by calling `Record` more times than the buffer holds (e.g. 200 calls for a 64-deep buffer) **with no receiver running**, then assert the recorder still answers: `Expect(recorder.Active(ctx, IdleCutoff)).To(ContainElement("abc"))`. Run the fill in a goroutine and `Eventually(filled).Should(BeClosed())` so a blocking send surfaces as a failed assertion rather than a suite timeout — a hung `Record` must fail this spec, not stall the run.

   c. **Every existing spec in the package keeps passing unchanged** — in particular in these three files, where the change is observable: `pkg/heartbeat/activity_test.go`, `pkg/heartbeat/publisher_test.go` and `pkg/heartbeat/integration_test.go`. (`observer_test.go` also exercises `Record` and must keep passing; it does, because the send is non-blocking and nothing reads the channel in that suite.) The two "idle past the cutoff" specs — `It("does not refresh a session that has been idle past the cutoff")` in `publisher_test.go` and `It("stops refreshing a session that has been idle past the cutoff")` in `integration_test.go` — record a session, advance the pinned clock past `IdleCutoff`, and then start the loop. They pass **because of requirement 2's activity check**: the drained notification is for a session that is no longer active, so `refreshOne` returns without writing and the count stays `0`. If either spec fails, the activity check is missing or misplaced — fix the check, do not relax the spec.

5. **Confirm the reader's contract is untouched.** The entry shape is still exactly `{"refreshedAt":"<RFC3339>"}` — `writer.go` is not modified by this prompt.

6. **Amend the README's timing sentence, and the matching `## Unreleased` CHANGELOG clause, because this change makes both inaccurate.** `README.md` currently says *"Every 20 seconds it stamps one entry per session it has served within the last 90 seconds…"*, which is no longer the whole truth.

   a. Add one sentence **immediately after the cadence sentence in that paragraph** — not at the end of the paragraph, whose last sentence is about RBAC — in the same voice:

   > A session's first entry is stamped the moment it is served, and the 20-second ticker then keeps it fresh while the session stays active.

   b. No new CHANGELOG bullet is needed, but the existing `## Unreleased` heartbeat bullet carries the **same** now-inaccurate claim, and it is the sentence that will ship as this feature's description. Extend that bullet with the clause `and a session's first entry is stamped the moment it is served`, so the shipped description matches the README.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `Record`'s send is in a `select` with a `default`, (ii) `Run`'s select has all three cases and the ticker case is still present, (iii) `observer.go` and `writer.go` are unchanged, and (iv) `go test -cover ./pkg/heartbeat/` still reports ≥80%.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT add a configuration knob, env var, or `application` field for the buffer size or the interval. The buffer capacity is an implementation constant in `activity.go`.
- Do NOT write to the ConfigMap from `Record`, from `observer.go`, or from any goroutine started per prompt. The publisher is the single writer and stays so.
- Do NOT make the channel send blocking, and do NOT use an unbuffered channel. A prompt must never wait on the heartbeat.
- Do NOT remove or lengthen the ticker. A session that stops being served must still age out within the reader's TTL.
- Do NOT change the entry shape, `ConfigMapName`, `RefreshInterval`, `IdleCutoff`, `Record`'s signature, or `Active`'s signature.
- No credential is written or logged — the entry is a session id and a timestamp and nothing else, and no log line this prompt touches may carry a token.
- Errors use `github.com/bborbe/errors` — never `fmt.Errorf`, never a bare `return err`. This prompt adds no new error path, so no new wrapping is expected.
- Logging uses `github.com/golang/glog`, lowercase messages.
- Tests use Ginkgo v2 / Gomega in the existing external `package heartbeat_test` suite. Do not add a second suite bootstrap.
- Run `make precommit` at the repository root. There is no per-package Makefile.
</constraints>

<verification>
1. `ROOTDIR=/workspace make precommit` — exits 0.
2. `ROOTDIR=/workspace go test -cover ./pkg/heartbeat/` — passes, and the printed coverage line reports ≥80% (a read-check: `go test -cover` prints the percentage but does not fail on it).
3. `grep -n 'Notifications() <-chan string' pkg/heartbeat/activity.go` — prints a line.
4. `grep -n 'case sessionID := <-p.recorder.Notifications()' pkg/heartbeat/publisher.go` — prints a line.
5. `grep -n 'case <-ticker.C' pkg/heartbeat/publisher.go` — prints a line (the ticker survived).
6. `grep -c 'default:' pkg/heartbeat/activity.go` — prints a count greater than `0` (the send is non-blocking).
7. `! grep -q 'writer' pkg/heartbeat/observer.go` — exits 0 (the observer still holds no writer).
8. `grep -n 'RefreshInterval = 20 \* time.Second' pkg/heartbeat/heartbeat.go` — prints a line (the cadence is unchanged).
</verification>
