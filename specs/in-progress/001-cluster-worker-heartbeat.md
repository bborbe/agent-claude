---
status: prompted
approved: "2026-10-05T16:07:54Z"
generating: "2026-10-05T16:10:15Z"
prompted: "2026-10-05T16:34:46Z"
branch: dark-factory/cluster-worker-heartbeat
---

## Summary

- A cluster worker — a Claude conversation inside the `claude-interactive` service — publishes its own liveness, so the supervisor's cluster liveness reader can see it.
- The service refreshes a timestamped entry per session it is actively serving in the `claude-worker-heartbeats` ConfigMap in nuke dev.
- The entry is refreshed on a cadence shorter than the 60-second TTL the reader applies, so a live worker never reads as stale.
- A worker that stops being addressed is not refreshed, so its entry ages out on its own — nothing has to clean it up, and a pod killed with no chance to clean up cannot leave a permanently-live entry.
- The shape written is the reader's own contract, not a shape this service prefers: the reader is not modified.

## Problem

The cluster liveness path reads a store that does not exist. `scripts/cluster-heartbeat.py` in `bborbe/claude-supervisor` reads the `claude-worker-heartbeats` ConfigMap in nuke dev, and no repo in the fleet writes it — `bborbe/claude-supervisor` holds exactly one reference to that name (the reader's own default at `scripts/cluster-heartbeat.py:65`) and no create/apply/patch anywhere. Measured 2026-10-05: `kubectlnukedev -n dev get configmap claude-worker-heartbeats` returns `NotFound`, `cluster-heartbeat.py --list --json` returns `[]`, and the poller's reachability marker reads `{"stamped":0}` — alive and stamping nothing.

So `pollCluster()` has nothing to mirror, `listLive` has no cluster entries to surface, and a live cluster worker is invisible to the fleet counter and to the sweep's liveness readers. Every downstream capability that depends on a cluster worker being counted is blocked on a write path that does not exist.

## Goal

The `claude-interactive` service in nuke dev publishes a fresh entry per actively-served session into the `claude-worker-heartbeats` ConfigMap, in the exact shape `scripts/cluster-heartbeat.py` consumes, so that `cluster-heartbeat.py --list --json` returns a real cluster worker while it is being served and stops returning it within one TTL of its last refresh once it stops. The end state is observable from outside the pod: a reader that today returns `[]` returns the worker.

## Non-goals

- Changing the reader (`scripts/cluster-heartbeat.py`), its 60-second TTL, or its `refreshedAt` contract. The reader is the source of truth and is not modified.
- Changing the local mirror (`server/cluster-heartbeat.mjs`) or the supervisor's `pollCluster()`.
- Building `claude-interactive` itself, beyond the heartbeat publishing described here.
- Changing the `github.com/bborbe/agent` library — no new surface is added there.
- Granting the RBAC the pod needs to write the ConfigMap — a manifest change in the config repo, named under Constraints as a required companion, not implemented here.
- The `spawnSync` timing interaction with the fleet cap — owned by its own row.
- The separate deploy gap where a session's local supervisor checkout predates the counter fix — filed separately; it affects whether a *given session* consumes the stamps, not whether they are written.

## Acceptance Criteria

- [ ] **Post-Deploy (Rung-2):** the service writes an entry into the `claude-worker-heartbeats` ConfigMap whose key is the session id it is serving and whose value is a JSON object carrying a `refreshedAt` field in RFC3339 form. Evidence: `kubectlnukedev -n dev get configmap claude-worker-heartbeats -o jsonpath='{.data}'` prints a map with the session id as a key and a `{"refreshedAt":"..."}` value.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** `cluster-heartbeat.py --list --json` returns the running worker. Evidence: `python3 scripts/cluster-heartbeat.py --list --json` (run from the `claude-supervisor` checkout) prints a non-empty JSON array containing an object with `session_id` equal to the served session id.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** while a worker is served, the reader returns it live at every sample across a window longer than one TTL. Evidence: **no further prompt is sent between the t=0 sample and the t=120 sample**, so this criterion isolates the refresh ticker rather than the prompt path; `python3 scripts/cluster-heartbeat.py --list --json` sampled at t=0, 30, 60, 90 and 120 s returns a non-empty array at **every** sample; equivalently, any two **consecutive** samples 30 s apart show a `refreshedAt` difference below 60 s. (The consecutive qualifier matters: the wider pairs t=0→t=90 and t=0→t=120 differ by more than 60 s by construction, because `refreshedAt` tracks wall-clock, so they are not evidence against the ticker.)
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** a session addressed, left idle past the idle cutoff, then addressed **again** is live after the second prompt. Evidence: `POST /prompt` on id `X`; wait ~200 s; `python3 scripts/cluster-heartbeat.py --list --json` returns `[]` for `X`; `POST /prompt` on `X` again; within one refresh interval `python3 scripts/cluster-heartbeat.py --list --json` contains `X` again. This is the criterion that separates per-prompt activity from first-touch-only registration.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** after the worker stops being served, the reader stops returning it within one TTL of the entry's last refresh. Evidence: `python3 scripts/cluster-heartbeat.py --list --json` returns an array that does not contain the session id, sampled after the entry has aged out. With the fixed numbers — refresh every 20 s, a 90 s idle cutoff, a 60 s reader TTL — the last prompt at t=0 is followed by refreshes at ≈20/40/60/80 s, the cutoff stops refreshing at 90 s, and the last stamp (≈t=80) keeps the reader returning the worker until ≈t=140; the sample is therefore taken at t ≥ 160 s and must not contain the id. (The bound is "gone by t=160", not "gone by t=140": the sample is deliberately taken a margin past the arithmetic so a jittered tick cannot make a correct implementation fail.)
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** a session that is never addressed has no entry — the writer publishes only for sessions it has actually served. Evidence: with the pod running and no prompt ever sent to id `X`, `kubectlnukedev -n dev get configmap claude-worker-heartbeats -o jsonpath='{.data.X}'` prints nothing.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** a pod killed with no chance to clean up does not leave a permanently-live entry. Evidence: after `kubectlnukedev -n dev delete pod claude-interactive-0 --wait=false` (⚠️ this mutates dev — it kills a running service pod), `python3 scripts/cluster-heartbeat.py --list --json` returns `[]` within 60 s.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] **Post-Deploy (Rung-2):** no credential value appears in the ConfigMap or in the service's log output. Evidence: `kubectlnukedev -n dev get configmap claude-worker-heartbeats -o jsonpath='{.data}' | grep -ci 'token\|secret\|bearer'` prints `0`, and `kubectlnukedev -n dev logs claude-interactive-0 | grep -ci 'ANTHROPIC_AUTH_TOKEN=\|Bearer '` prints `0`.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.7.0`
- [ ] The reader's own source is unmodified. Evidence: `git -C ~/Documents/workspaces/claude-supervisor diff --stat scripts/cluster-heartbeat.py` prints nothing.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` — exits 0.
- `make test` — the unit and integration suites pass.
- `go build ./...` — the new k8s client dependency compiles.
- `grep -n 'refreshedAt' <changed files>` — the writer emits the field the reader parses.
- The integration test asserts the emitted `data` value round-trips through a JSON parse and yields a `refreshedAt` string that parses as RFC3339 — a behavioral assertion, not a literal-count one.

### Operator-executable (runs on the host after the image lands in dev)

All of these read or mutate the live nuke-dev cluster and the `claude-supervisor` checkout on the operator's machine.

- `kubectlnukedev -n dev get configmap claude-worker-heartbeats` — present after the worker runs.
- `python3 scripts/cluster-heartbeat.py --list --json` (run from `~/Documents/workspaces/claude-supervisor`) — returns the worker while it is served, and `[]` within 60 s of it stopping.
- `kubectlnukedev -n dev logs claude-interactive-0` — shows the writer started, with no credential in the line.

## Desired Behavior

1. **The writer publishes per served prompt.** Each time the service completes a prompt on a session id, that id becomes publishable; the writer refreshes an entry for it keyed by that exact id. Activity is observed by wrapping the **`Session`** the factory returns and recording the id in its `Prompt` method — **not** by wrapping the factory's `Create`, which the library calls only on first use and never again.
2. **The entry carries the reader's field.** The value is a JSON object with a `refreshedAt` field in RFC3339 form, stamped at write time. Nothing else in the value is required by the reader, and no other field is added that the reader would have to parse.
3. **The refresh cadence is shorter than the TTL.** The writer refreshes on a fixed interval of 20 seconds, against the reader's 60-second TTL — a margin of three refreshes per TTL window, so a single missed tick cannot make a live worker read as stale.
4. **Only recently-served sessions are refreshed.** A session that has not completed a prompt for 90 seconds stops being refreshed. Its entry then ages out of the reader's view on its own, so an idle conversation does not read as a live worker.
5. **The write is a merge, not a replace.** The writer patches only its own keys, so two pods or two writers cannot erase each other's entries.
6. **A failing cluster write does not take the service down.** If the ConfigMap write fails, the writer logs the failure and continues serving prompts; the liveness entry goes stale and the reader reports the worker dead, which is the correct answer for a worker whose liveness path is broken.
7. **The service's existing surface is unchanged.** `/readiness`, `/metrics`, `/prompt` and the permission endpoint keep their current behavior, their authentication, and their response shapes.

## Constraints

- **The reader is not modified.** Its contract at `scripts/cluster-heartbeat.py:54-65` and `:113-131` is the source of truth: `data` maps `<session_id>` to a JSON string carrying `refreshedAt` (RFC3339); the verdict is the stamp's age against `TTL_SECONDS = 60`; an absent ConfigMap is a readable-and-empty answer, not an error. The writer satisfies this contract rather than the reader being bent to match the writer.
- **The identity is the session id the service is serving** — the value the caller supplies as `X-Session-Id`, which is also what `bborbe/claude-supervisor`'s `cluster-spawn.mjs` mints with `randomUUID()` and records as the ledger's `session_id`. It is not a pod name and not a task id.
- **The change stays in this repository.** The `Session` interface is two methods (`Prompt`, `Close`) and `SessionFactory` is one (`Create(id string) Session`) — both frozen external contracts of `github.com/bborbe/agent`. The activity signal is captured by wrapping the `Session` this binary's factory returns; no change to that library is required, and none is made.
- **The pod needs RBAC to write the ConfigMap.** This repository cannot grant it: the `claude-interactive` pod runs under the chart's service account, and the permission is a manifest change in the config repo. **This is a required companion change and must land with or before this image** — without it every write fails with `configmaps is forbidden` and the feature is inert while looking deployed.
- **`docs/dod.md` applies.** A README note for the new cluster-write behavior and a `## Unreleased` CHANGELOG entry are part of the change. The README note must state the writer-side contract, because it is domain knowledge that outlives this spec: the ConfigMap name (`claude-worker-heartbeats`), the key (the session id), the value (`{"refreshedAt": "<RFC3339>"}`), the 20 s refresh cadence, and the 90 s idle cutoff.
- **No credential is written or logged.** The ConfigMap carries a session id and a timestamp and nothing else. The writer must not log the bearer token, and must not add the token or any environment value to the entry.
- **Existing behavior is preserved.** The task-routed path (Kafka jobs) does not serve HTTP and gains no writer. The service's authenticated routes keep their contract.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---|---|---|---|
| The cluster API is unreachable from the pod | The write fails; the service keeps serving; the entry goes stale and the reader reports the worker dead | Restore API reachability; the next tick refreshes the entry | Writer logs the write failure; `cluster-heartbeat.py --list --json` returns `[]` while the pod is Running |
| The pod is killed with no cleanup | No further refreshes; the entry ages out of the reader's view within 60 s | None needed — the age-based verdict is the mechanism | `cluster-heartbeat.py --list --json` stops returning the id within 60 s |
| Two writers write concurrently | Each patches only its own keys; neither erases the other's | None needed | Both session ids present in `.data` |
| The RBAC grant is missing | Every write fails with `configmaps is forbidden`; the feature is inert | Apply the config-repo RBAC companion | Writer logs the forbidden error; `.data` stays empty |
| The writer's `data` shape drifts from the reader's parser | The reader silently drops every entry (an unparseable value is skipped, not an error) | Restore the shape; the next tick refreshes | `.data` is populated while `cluster-heartbeat.py --list --json` returns `[]` |
| The clock is skewed | `refreshedAt` is in the future or far past; the reader drops the entry (`age < 0` or `age >= ttl`) | Resync the node clock and confirm `date -u` is within 1 s of the API server; the next tick re-stamps `refreshedAt` | `cluster-heartbeat.py --list --json` omits a pod that is Running and writing |
| The ConfigMap does not exist yet | The write creates it | Confirm `.data` is readable after the first tick; if it is absent, the write path is patch-only and must be changed to create-if-absent | `.data` readable after the first tick |

## Security / Abuse

- The ConfigMap is a **liveness signal, not a trust boundary**. It carries a session id and a timestamp; a forged entry can make a non-existent worker look live, which at worst delays a resume decision. Nothing authenticates on this store and nothing should.
- The session id is written verbatim as a ConfigMap key. It must therefore be validated against the service's existing session-id pattern before use as a key, because a key that escapes the pattern could collide with another entry or make the ConfigMap invalid. The service already validates it at intake (`github.com/bborbe/agent/interactive/session-id.go` — a path in the **library**, not in this repository), so the writer inherits a validated value and must not accept an unvalidated one.
- **No secret value may reach this store.** The entry is a session id and a timestamp. The bearer token, the Anthropic credentials and every other environment value stay out.

## Suggested Decomposition

| # | Prompt focus | Covers Desired Behaviors | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Session-activity observer (wrapping `Session.Prompt`) + heartbeat writer behind an interface, with unit tests | 1, 2, 3, 5, 6 | 1, 6, 8 | — |
| 2 | Wire the writer into `runService` with an in-cluster k8s client and the ticker | 4, 7 | 2, 4, 5, 7 | prompt 1 |
| 3 | Integration test driving the writer against a fake cluster surface; assert the emitted `data` shape parses through the reader's own parser | 2, 3 | 1, 3 | prompt 1 |

Rationale: prompt 1 establishes the writer and its contract in isolation; prompt 2 is the wiring that makes it live; prompt 3 locks the shape against the reader's parser so a drifting `data` shape fails a test rather than a silent cluster read.

AC9 (the reader is unmodified) is deliberately unmapped: it is a cross-cutting invariant that holds across all three prompts rather than being owned by any one of them. Each prompt must leave `scripts/cluster-heartbeat.py` untouched.

**Size note:** this is a 7-Desired-Behavior / 9-AC spec — above the 8×8 guidance when multiplied (63 > 50). It is left whole deliberately: the three prompts above are the natural seam and the layers are shallow (one new package, one wiring point, one test), so splitting would add an approval round without reducing what any single prompt must hold.

**No new scenario.** The runtime-only failure this spec names (a missing RBAC grant producing `configmaps is forbidden`) is exercised by the Post-Deploy Rung-2 ACs against the real cluster, which a test double cannot fake and which the operator ladder already runs. No `scenarios/` directory exists in this repo, and adding one would duplicate coverage the Post-Deploy rung provides.

## Do-Nothing Option

Do nothing and the cluster liveness store stays empty. A live cluster worker remains invisible to the fleet counter and to every liveness reader — which is the defect this work exists to remove — and every downstream capability that depends on counting a cluster worker stays blocked. The cost of doing nothing is that the capability the parent goal exists to deliver cannot be counted, and the operator cannot tell a running cluster worker from a dead one.
