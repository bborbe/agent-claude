---
status: approved
spec: [005-a2a-worker-status-push]
created: "2026-10-17T16:04:54Z"
queued: "2026-10-18T08:34:50Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. BLOCKING COMPANION DEPENDENCY — like the first prompt in this batch, this test
   cannot pass until `github.com/bborbe/agent` lands BOTH companion changes named in
   the spec's Constraints: the `interactive` package's push extension point (store,
   sender, card capability, `a2asrv.WithPushNotifications` in `a2a-handler.go`) AND
   the mid-turn working-state status in `a2aExecutor.Execute` (spec row 2). At
   `agent@v0.99.1` the executor yields exactly submitted-task -> artifact ->
   terminal-status, so there is no working-state event to observe and the mid-turn
   half of this test has nothing to find. Requirement 1 makes the test fail loudly
   and name the gap rather than passing vacuously; requirement 2 is what stops the
   test from hanging for its whole timeout when the event never arrives.

2. SETTLED — the turn window is read from the worker's OWN `turn start id=<id>` /
   `turn end id=<id>` glog lines (the base AC 3 and AC 4 name), not from the test's
   own clock around the HTTP POST. glog's stderr header carries no year, so the
   helper reconstructs it from `time.Now()`; a marker that is missing or unparseable
   FAILS the spec rather than skipping the assertion, because a silently unparsed
   marker would turn the whole window check into a no-op.

3. SETTLED — the task id is chosen by the test and sent in `message.taskId`, because
   the SDK's local manager derives the task id from that field
   (`internal/taskexec/local_manager.go:153-163`: `tid = req.Message.TaskID` when set,
   else a fresh id). Without a client-chosen id the test cannot name the task to
   register a callback for while the turn is still running, which is the only moment
   a registration can be accepted — the SDK refuses one naming a task its task store
   does not hold, and refuses a follow-up message on a task already in a terminal
   state (`internal/taskexec` via `factory.loadExecutionContext`), so a
   turn-then-register-then-second-turn sequence is impossible.

4. SETTLED — the absent-token assertion reads the RAW header map keyed by
   `http.CanonicalHeaderKey("A2A-Notification-Token")` rather than
   `Header.Get(...) == ""`. `Get` cannot tell an absent header from an empty-string
   one, and "an absent token is not an empty-string token" is precisely what AC 5's
   negative half asserts. Note the canonical form of that header name is
   `A2a-Notification-Token`, not `A2A-Notification-Token`.

5. SETTLED — `main_test.go`'s suite timeout is raised. It is the Ginkgo suite
   bootstrap (`suiteConfig.Timeout = 60 * time.Second`), not one of the route /
   authentication / session-cache contract tests AC 9 requires to pass unmodified,
   and this batch adds a spec that deliberately runs a ~35-second turn. Leaving the
   ceiling at 60s would abort the suite mid-turn on a slow runner and report a
   timeout instead of the assertion that actually failed.

6. OUT OF SCOPE — the operator-executable half of the spec's Verification ladder
   (build an image from HEAD, run it with `host.docker.internal`, read the two POSTs
   off a host receiver) is not in this prompt: the container has no Docker socket.
   It stays on the spec. The k8s arm is out of scope for the same reason as in
   prompt 1.
-->

# Lock the A2A push wire behaviour against a loopback receiver

<summary>
- A worker accepts a callback registration for a task it holds and returns it readable back, and refuses one naming a task it has never seen.
- One turn delivers a status update while the turn is still running and a completion when it ends, both to the registered address.
- The mid-turn update is proven to land in the middle of the turn — not at its first instant and not at its last.
- The worker's turn does not get longer because of the status it emits.
- The pushed body is the A2A task-update shape rather than a bespoke object, and the registered token travels in the protocol's own header.
- A registration that carries no token produces a request with no token header at all, rather than one with an empty value.
- The same push is refused when the sender's default private-network guard is left on, and delivered when it is opted out — the test proves the opt-out is what makes the channel work.
- The test drives the real service the worker builds, with the real sender and a real HTTP receiver, rather than a mock of either.
- Nothing in the worker's production behaviour changes: this prompt adds test files only.
- The library change this depends on, the spawner side and the controller side are explicitly not part of this change.
</summary>

<objective>
Add an in-process integration test that drives the worker's own interactive service — the one `newInteractiveService` builds, with the real push store and the real HTTP sender — against a receiver bound to loopback, and asserts the wire behaviour end to end: the card advertises push support, a registration round-trips for an existing task and is refused for an unknown one, one turn yields a mid-turn working-state status inside the turn's own window plus a completion at or after its end, the pushed body is the SDK's stream-response envelope, the registered token travels as the `A2A-Notification-Token` header (and no header at all when the config carries none), and the same push is refused when the sender's private-network guard is left at its default. End state: a future regression in the push wiring fails a test here instead of silently producing a worker that looks deployed and pushes nothing.
</objective>

<context>
Read `docs/dod.md` — the Definition of Done you are graded against.

**The blocking prerequisite.** The A2A handler, the Agent Card and the executor's event sequence all live in `github.com/bborbe/agent`'s `interactive` package. This test needs BOTH companion changes named in the spec's Constraints: the package must accept the push store and sender and apply `a2asrv.WithPushNotifications(...)` in `a2a-handler.go`, and `a2aExecutor.Execute` must yield the mid-turn working-state status. At `v0.99.1` the executor yields exactly this:

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/a2a-handler.go
if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
	return
}
result, err := e.cache.Get(ctx, sessionID).Prompt(ctx, prompt)
// ...
if !yield(a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(result)), nil) {
	return
}
yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
```

There is no working-state event there, so the mid-turn half of this test has nothing to observe until the companion lands. Requirement 1 makes the test name that gap rather than pass vacuously. **This repository cannot make the library change** — it is a dependency. If the companion has not landed, stop and report rather than writing a test that cannot pass.

Files you will read and change (repo-relative):
- `main.go` — the `application` struct and `newInteractiveService`, which is the service under test. Do not change it in this prompt; the first prompt in this batch owns it.
- `main_internal_test.go` — `package main`, and the pattern this file follows: it already builds the service via `app.newInteractiveService(&agentmocks.SessionFactory{}, prometheus.NewRegistry(), interactive.NewPermissionRegistry())` and serves `service.Handler()`. Read it before writing.
- `main_test.go` — the suite bootstrap: `TestSuite` calls `RunSpecs` with `suiteConfig.Timeout = 60 * time.Second`. Requirement 4 raises that ceiling. No other change to this file.
- `$(go env GOMODCACHE)/github.com/bborbe/agent@<version>/interactive/service_test.go` — the library's own spec file. It shows the two mechanisms this test borrows: the `BeforeSuite` that raises glog verbosity (`flag.Set("logtostderr", "true")` then `flag.Set("v", "2")`) and `captureStderr`, which swaps `os.Stderr` for a pipe while `fn` runs and returns what was written. glog's stderr sink reads `os.Stderr` when it emits, so the swap is observed as long as `-logtostderr` is on.

The turn markers this test measures against, and why they need `-v=2`:

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/session-cache.go
glog.V(2).Infof("turn start id=%s", e.id)
result, err := e.session.Prompt(ctx, prompt)
glog.V(2).Infof("turn end id=%s", e.id)
```

glog's stderr header is `"<severity><MMDD> <HH:MM:SS.microseconds> <pid> <file>:<line>] <message>"` — for example
`I1017 17:00:00.123456   12345 session-cache.go:73] turn start id=pushprobe` — and carries no year, so the helper reconstructs it.

The library API the test drives, quoted from the module cache:

```go
// github.com/bborbe/agent — import paths github.com/bborbe/agent and .../interactive, .../mocks
type Session interface {
	Prompt(ctx context.Context, prompt string) (string, error)
	Close(ctx context.Context) error
}

type SessionFactory interface {
	Create(id string) Session
}

// mocks.SessionFactory / mocks.Session are the counterfeiter fakes:
//   factory.CreateStub = func(id string) agentlib.Session { ... }
//   session.PromptStub = func(ctx context.Context, prompt string) (string, error) { ... }

// interactive — the service under test
func NewPermissionRegistry() PermissionRegistry
func (s Service) Handler() http.Handler
```

The session id the A2A request names must match the library's own anchored pattern, `^[A-Za-z0-9_][A-Za-z0-9_-]{0,63}$` — use a plain id such as `pushprobe`, not a dotted or slashed one.

The SDK surface the assertions decode. Quoted verbatim from `$(go env GOMODCACHE)/github.com/a2aproject/a2a-go/v2@v2.6.0`:

```go
// a2a/core.go — the pushed body is this union, marshalled with exactly one key set
type StreamResponse struct {
	Event
}
// MarshalJSON writes exactly one of: "message", "task", "statusUpdate", "artifactUpdate"

// a2a/core.go — a text part flattens to {"text": "..."}
func NewTextPart(text string) *Part
func NewMessage(role MessageRole, parts ...*Part) *Message

// a2a/core.go
type TaskStatusUpdateEvent struct {
	ContextID string     `json:"contextId" ...`
	Status    TaskStatus `json:"status" ...`
	TaskID    TaskID     `json:"taskId" ...`
}
type TaskStatus struct {
	Message   *Message   `json:"message,omitempty" ...`
	State     TaskState  `json:"state" ...`
	Timestamp *time.Time `json:"timestamp,omitempty" ...`
}
// a2a/core.go: TaskStateWorking = "TASK_STATE_WORKING", TaskStateCompleted = "TASK_STATE_COMPLETED"

// a2asrv/push — import under an alias, e.g. a2apush (main.go already imports the
// prometheus push package as `push`)
func NewHTTPPushSender(config *HTTPSenderConfig) *HTTPPushSender
type HTTPSenderConfig struct {
	Timeout              time.Duration
	FailOnError          bool
	AllowPrivateNetworks bool
}
func (s *HTTPPushSender) SendPush(ctx context.Context, config *a2a.PushConfig, event a2a.Event) error

// a2a/push.go
type PushConfig struct {
	Tenant string        `json:"tenant,omitempty" ...`
	TaskID TaskID        `json:"taskId" ...`
	ID     string        `json:"id,omitempty" ...`
	Auth   *PushAuthInfo `json:"authentication,omitempty" ...`
	Token  string        `json:"token,omitempty" ...`
	URL    string        `json:"url" ...`
}
```

The JSON-RPC wire surface, verbatim from `$(go env GOMODCACHE)/github.com/a2aproject/a2a-go/v2@v2.6.0/internal/jsonrpc/jsonrpc.go`:

```
method names:  "SendMessage", "GetTask", "CreateTaskPushNotificationConfig",
               "GetTaskPushNotificationConfig"
error codes:   -32001 TASK_NOT_FOUND, -32003 PUSH_NOTIFICATION_NOT_SUPPORTED
```

The registration and read-back bodies (the params are the `PushConfig` and `GetTaskPushConfigRequest` structs, unmarshalled straight from `params`):

```json
{"jsonrpc":"2.0","id":1,"method":"CreateTaskPushNotificationConfig","params":{"taskId":"push-task-1","url":"<receiver>","token":"push-token"}}
{"jsonrpc":"2.0","id":2,"method":"GetTaskPushNotificationConfig","params":{"taskId":"push-task-1","id":"<id from the create response>"}}
```

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-test-types-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Prove the companion landed before writing assertions against it.** The push wiring is the service's own construction (the first prompt in this batch), but the *mid-turn event* and the *card capability* come from the library. So, before writing the test:

   a. Read `$(go env GOMODCACHE)/github.com/bborbe/agent@<the version pinned in go.mod>/interactive/a2a-handler.go` and confirm `a2aExecutor.Execute` yields a working-state status between the submitted task and the artifact — `a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, <non-nil message>)`, or an equivalent that lands in the same position.

   b. Read the same version's `a2a.go` and `a2a-handler.go` and confirm the card capability and `a2asrv.WithPushNotifications(...)` are wired there.

   c. **If either is missing, STOP and report** — naming which one, and stating that the companion change must land first (spec Constraints, "Required companion change in `github.com/bborbe/agent`", and spec row 2). Do not write the test against an event that does not exist, do not implement the library here, do not fork it, and do not add a `replace` directive.

2. **Add the integration test file `main_a2a_push_internal_test.go`, `package main`,** with the repository's BSD licence header (`// Copyright (c) 2026 Benjamin Borbe All rights reserved.` / `// Use of this source code is governed by a BSD-style` / `// license that can be found in the LICENSE file.`), so `make precommit`'s `addlicense` step has nothing to rewrite. All specs below live in this one file, in `package main` (not `main_test`) because `application` and its `newInteractiveService` method are unexported — the same reason `main_internal_test.go` gives for its own package clause. `RunSpecs` already lives in `main_test.go`; do not add a second one.

   **Glog setup.** Add a `BeforeSuite` that turns on glog's stderr sink and raises the verbosity the turn markers need:

   ```go
   var _ = BeforeSuite(func() {
       Expect(flag.Set("logtostderr", "true")).To(Succeed())
       Expect(flag.Set("v", "2")).To(Succeed())
   })
   ```

   `flag.Set` must run before any spec, because the argument-parsing specs in `main_internal_test.go` replace `flag.CommandLine` in a `BeforeEach` — after that replacement glog's flags are no longer registered and `flag.Set` would fail. A `BeforeSuite` always runs first, so this is the only place the calls can live. Do not add a second `BeforeSuite` elsewhere in the file.

   **The stderr capture helper.** Reproduce the library's mechanism (do not import it — it is unexported there):

   ```go
   // captureStderr redirects the process's stderr into a buffer for the duration of fn
   // and returns what was written. glog's stderr sink reads os.Stderr when it emits, so
   // the swap is observed as long as -logtostderr is on.
   func captureStderr(fn func()) string { /* os.Pipe + io.Copy in a goroutine + swap os.Stderr */ }
   ```

   **The glog header parse.** glog's header is `"<severity><MMDD> <HH:MM:SS.microseconds> <pid> <file>:<line>] <message>"` and carries no year:

   ```go
   // glogLineTime parses the timestamp out of a glog stderr line. glog's header carries
   // no year, so the year is taken from now: time.Parse with layout "0102 ..." leaves the
   // year at 0, and AddDate(now.Year()-parsed.Year(), 0, 0) sets it to the current one.
   func glogLineTime(line string, now time.Time) (time.Time, bool) {
       if len(line) < 21 || line[0] != 'I' {
           return time.Time{}, false
       }
       parsed, err := time.Parse("0102 15:04:05.000000", line[1:21])
       if err != nil {
           return time.Time{}, false
       }
       return parsed.AddDate(now.Year()-parsed.Year(), 0, 0), true
   }

   // turnWindow scans captured stderr for the two turn-boundary lines of one session id
   // and returns their parsed instants. It fails the spec — never skips it — when a
   // marker is absent or unparseable, because a silently unparsed marker would turn the
   // whole mid-turn window check into a no-op.
   func turnWindow(output, sessionID string, now time.Time) (start, end time.Time) { /* ... */ }
   ```

   `turnWindow` must find the lines whose message text is exactly `turn start id=<sessionID>` and `turn end id=<sessionID>`, and if there is more than one such pair (a second turn on the same id), use the pair belonging to the long turn — the test runs exactly one turn on that id, so assert there is exactly one of each and say so in the failure message if not.

   **The receiver.** A recording HTTP receiver, bound to loopback by construction (`httptest.NewServer`):

   ```go
   // recordedPush is one POST the receiver observed: when it arrived, the headers it
   // carried and its raw body.
   type recordedPush struct {
       arrived time.Time
       headers http.Header
       body    []byte
   }

   // pushReceiver records every POST it is sent, under a mutex, in arrival order. It
   // answers 200 so the sender treats the push as delivered.
   type pushReceiver struct {
       mutex sync.Mutex
       posts []recordedPush
   }
   func newPushReceiver() *pushReceiver            // httptest.NewServer(receiver.handler())
   func (r *pushReceiver) record(...)              // copies r.Header (it is only valid for the request) and the body
   func (r *pushReceiver) all() []recordedPush     // a copy, under the mutex
   func (r *pushReceiver) server() *httptest.Server
   ```

   **The service under test.** Build it through the worker's own constructor, so the test drives the real wiring rather than a hand-assembled service:

   ```go
   app := &application{
       Listen:               "127.0.0.1:0",
       InteractiveAuthToken: testPushToken,     // const testPushToken = "test-push-token"
       A2APublicURL:         "https://agent.example.test/a2a",
       A2AAgentName:         "claude-interactive",
   }
   service := app.newInteractiveService(sessions, prometheus.NewRegistry(), interactive.NewPermissionRegistry())
   server := httptest.NewServer(service.Handler())
   ```

   `sessions` is an `*agentmocks.SessionFactory` whose `CreateStub` returns an `*agentmocks.Session` whose `PromptStub` sleeps and then returns a fixed string. Every request to `/a2a` carries `Authorization: Bearer ` + `testPushToken`; the route is gated, and without the header the gate answers 401 before the handler runs.

   **The request bodies.** Write small helpers rather than inlining JSON: `a2aCallBody(id int, method string, params any) string` marshalling `{"jsonrpc":"2.0","id":id,"method":method,"params":params}`, and `postA2A(serverURL, body string) (int, string, error)` sending it with `Content-Type: application/json` and the bearer token. Decode responses into an envelope carrying `Result json.RawMessage` and `Error *struct{ Code int; Message string }` so a spec can assert on either half.

3. **Add the cheap sender-level specs.** These need no turn and no service: they drive the SDK's own sender directly against the receiver, and they are what makes the guard and the token assertions precise. Put them in a `Describe("A2A push sender")`.

   a. **Production config delivers, and the token travels as the protocol header (AC 5 positive, AC 6 positive).** Build `a2apush.NewHTTPPushSender(&a2apush.HTTPSenderConfig{AllowPrivateNetworks: true})` — the production configuration — and call `SendPush(ctx, &a2a.PushConfig{URL: receiver.server().URL, Token: "push-token"}, event)` with a `*a2a.TaskStatusUpdateEvent` you construct directly. Expect no error, exactly one recorded POST, and `recorded.headers.Get("A2A-Notification-Token")` equal to `"push-token"`.

   b. **No token in the config means no token header at all (AC 5 negative).** Same sender, same call, but `&a2a.PushConfig{URL: receiver.server().URL}` with no `Token`. Expect no error, exactly one recorded POST, and the header **absent** — assert on the raw map:

      ```go
      _, present := recorded.headers[http.CanonicalHeaderKey("A2A-Notification-Token")]
      Expect(present).To(BeFalse())
      ```

      Do NOT write this as `Expect(recorded.headers.Get("A2A-Notification-Token")).To(BeEmpty())`: `Get` cannot tell an absent header from an empty-string one, and "an absent token is not an empty-string token" is exactly what this half asserts. Note also that the canonical form of the header name is `A2a-Notification-Token`, which is why `http.CanonicalHeaderKey` is used on both sides.

   c. **The default guard refuses a loopback target (AC 6 negative, production shape).** Build the sender with the guard left at its default (`&a2apush.HTTPSenderConfig{}`, i.e. `AllowPrivateNetworks: false` and `FailOnError: false`) and call `SendPush` against the loopback receiver. Expect the call to return **no error** — that is the spec's invariant that a push failure is logged and swallowed and never returned to the caller — and the receiver to have recorded **nothing**.

   d. **The refusal is the private-network guard, not a network hiccup (AC 6 negative, mechanism).** Same default-guard sender, but with `FailOnError: true`, so the refusal surfaces as a returned error instead of being swallowed. Expect the error to name the blocked range: `MatchError(ContainSubstring("blocked address range"))` (the SDK's own text is `push notification target resolves to a blocked address range`). Together with spec c this proves both halves of the AC's sentence: the refusal happens, and with `FailOnError` false it is swallowed rather than returned.

   e. **The pushed body is the A2A envelope, not a bespoke shape (AC 7).** On spec a's recorded POST, decode the body into `map[string]any` and assert it has **exactly one** top-level key and that the key is `statusUpdate`; then decode `body["statusUpdate"]` into a struct carrying `taskId` and `status.state` and assert both are the values you sent. A body keyed on `event`, or a hand-rolled `{worker_id, state, text, ts}` object, fails this. Add a comment stating that the single-key assertion is the discriminator check, and that the SDK's `StreamResponse.MarshalJSON` writes exactly one of `message` / `task` / `statusUpdate` / `artifactUpdate`.

4. **Add the long-turn spec (ACs 2, 3, 4, 6, 8, 11).** One spec, in a `Describe("A2A push notification end to end")`, because the turn it runs takes about 35 seconds and the suite must not run it more than once.

   Constants, each with a comment naming where the value comes from:

   ```go
   // longTurn is the turn's duration. It must exceed 30 seconds so the fixed mid-turn
   // emission (10 seconds after the turn starts) lands with room on both sides: at least
   // 10 seconds after the turn start and at least 10 seconds before the turn end.
   const longTurn = 35 * time.Second

   // midTurnDelay mirrors the companion's fixed midTurnStatusDelay (10 * time.Second).
   // It is a mirror for the assertion's arithmetic only — it is not a knob, and nothing
   // in this repository configures it.
   const midTurnDelay = 10 * time.Second
   ```

   The spec body, in order:

   a. `sessionID := "pushprobe"`, `taskID := "push-task-1"`, `pushToken := "push-token"`. Build the receiver, the fake session factory (its `PromptStub` sleeps `longTurn` and returns a fixed string), the service and the `httptest.Server`. `DeferCleanup` the server and the receiver.

   b. Run everything that must happen during the turn inside one `captureStderr(func() { ... })` call, so the two glog boundary lines are captured:

      1. Start the turn in a goroutine: `go func() { done <- postSendMessage(...) }()` where the body is a `SendMessage` whose `message` is
         `{"messageId":"m1","role":"ROLE_USER","contextId":"pushprobe","taskId":"push-task-1","parts":[{"text":"hello"}]}`.
         The `taskId` is load-bearing: the SDK's local manager takes the task id from it, and the test cannot register a callback for a task whose id it does not know (see the reviewer note on why a second turn on the same task is impossible).
      2. Wait for the task to exist before registering: poll `GetTask` with `{"id":"push-task-1"}` until the response carries a `result` rather than an `error`, with a generous ceiling (`Eventually(..., 20*time.Second, 100*time.Millisecond)`). Before the task exists the call answers `TASK_NOT_FOUND`.
      3. Register the callback: `CreateTaskPushNotificationConfig` with `{"taskId":"push-task-1","url": receiver.server().URL, "token": pushToken}`. Expect a result, no error, and capture the returned `id`.
      4. Read it back: `GetTaskPushNotificationConfig` with `{"taskId":"push-task-1","id": <the id from step 3>}`. Expect the same `url` and the same `token` — this is AC 2's round-trip half.
      5. Refuse an unknown task: `CreateTaskPushNotificationConfig` with `{"taskId":"never-seen","url": receiver.server().URL, "token":"tok"}`. Expect an error whose body carries `TASK_NOT_FOUND` (code `-32001`) — this is AC 2's refusal half, and the reason string is what distinguishes it from the `PUSH_NOTIFICATION_NOT_SUPPORTED` refusal the first prompt's fail-closed spec asserts.
      6. Wait for the turn to finish: `Eventually(done, 2*longTurn).Should(Receive(BeNil()))`. A nil error means the turn completed normally, which is also the evidence that the push never failed the turn.

     Register promptly after step 2 — the mid-turn emission is 10 seconds into the turn, so the registration has roughly that much slack. A registration that is refused must fail the spec loudly (step 3's `Expect(err).NotTo(HaveOccurred())` does that) rather than being logged and skipped, because a skipped registration makes every later assertion vacuous.

   c. Parse the turn window out of the captured stderr with `turnWindow(output, sessionID, time.Now())`, and assert both markers were found (the helper fails the spec when they are not).

   d. Assert on `receiver.all()`:
      - **the two status updates are selected by state, not by count (AC 3).** Collect the recorded POSTs whose body decodes to a `statusUpdate`; there must be at least two. Among them there must be exactly one whose `status.state` is the SDK's own working-state constant — `string(a2a.TaskStateWorking)`, i.e. `TASK_STATE_WORKING` on the wire — and one whose state is `string(a2a.TaskStateCompleted)`. Compare against those constants rather than against a hand-typed literal. Do not assert a total POST count: the SDK pushes one POST per task event, so the turn also produces the submitted-task and artifact events, and their number is not this test's contract.
      - **the mid-turn status lands inside the turn (AC 3, AC 4).** The working-state POST's `arrived` is strictly after `start`, strictly before `end`, at least `midTurnDelay - 1*time.Second` after `start`, and at least `midTurnDelay` before `end`; and its body's message text is non-empty (decode `statusUpdate.status.message.parts[0].text`, which is where a text part flattens to). The last two bounds are what reject a send fired at the turn's first instant or its last. ⚠️ The **`- 1*time.Second` slack on the `after start` bound is deliberate and not a weakening**: `turn start` is emitted inside the `Prompt` goroutine while the delay timer is armed on the executor's goroutine, so the two instants are separated by sub-second scheduling skew and a full-`midTurnDelay` floor fails intermittently on a correct implementation. It matches spec 005 AC 4's 9 s floor and `bborbe/agent` spec 061's AC 7. The `before end` bound keeps the full `midTurnDelay`, because there the delay works in the assertion's favour.
      - **the completion lands at or after the turn end (AC 3).** The completed-state POST's `arrived` is not before `end`.
      - **the token header (AC 5).** The working-state POST's `A2A-Notification-Token` header equals `pushToken`.
      - **the envelope (AC 7).** Every status POST's body has exactly one top-level key, `statusUpdate`, and its `taskId` equals `taskID`.
      - **the turn was not lengthened (AC 11).** `end.Sub(start)` is less than `longTurn + midTurnDelay/2` — that is, closer to the work's own duration than to the work plus the mid-turn delay. A `sleep; yield; work` implementation, which pushes the turn's end out by the delay while still landing inside the turn window, fails this and nothing else.

   e. Do not assert anything about wall-clock durations other than the two above, and do not compare the worker's clock against the test's: the window is measured against the worker's own two lines, which share its clock, so a skewed host cannot break the check.

5. **Raise the suite timeout in `main_test.go`.** The suite now contains a spec that deliberately runs a ~35-second turn, and the existing ceiling was chosen when every spec was sub-second. Change `suiteConfig.Timeout = 60 * time.Second` to `5 * time.Minute`, with a one-line comment stating why (a spec that runs a deliberately long turn must not be aborted mid-turn by the suite ceiling and reported as a timeout instead of as its real assertion). This is the Ginkgo suite bootstrap, not one of the route / authentication / session-cache contract tests the spec's AC 9 requires to pass unmodified — those files are untouched by this prompt.

6. **Change no production file.** This prompt adds one test file (`main_a2a_push_internal_test.go`) and changes one line plus one comment in `main_test.go`. Do not touch `main.go`, `main_internal_test.go`, `pkg/**`, `mocks/**`, `README.md` or `CHANGELOG.md` — the first prompt in this batch owns all of those, including the changelog entry.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the test you actually wrote. In particular confirm (i) the absent-token assertion reads the raw header map rather than `Get`, so an empty-string token would fail it; (ii) the mid-turn bounds are asserted against the parsed `turn start` / `turn end` lines and not against the test's own clock around the POST; (iii) the spec fails rather than skips when a marker is missing or a registration is refused; (iv) no total-POST-count assertion exists; and (v) no production file was modified.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Run `make` at the repository root, and pass `ROOTDIR=/workspace` on every `make` call in this container.
- This prompt is test-only. `main_a2a_push_internal_test.go` is the only new file; `main_test.go` gains one changed value and one comment. No production file changes.
- **The library change is a dependency, not a deliverable.** The push extension point and the mid-turn working-state status both live in `github.com/bborbe/agent`'s `interactive` package and are that repository's work — hand-implemented or driven by its own spec. Do NOT implement either here, do NOT fork the library, do NOT vendor it, and do NOT add a `replace` directive.
- **The spawner and the controller are out of scope.** `bborbe/claude-supervisor` receives and surfaces the pushed status and `bborbe/a2a-worker-controller` registers the callback; both are hand-implemented from the parent task and neither checkout is mounted in this container. Do not attempt to modify either.
- The k8s arm is out of scope. The spec's operator-executable rung (build an image from HEAD, run it with `host.docker.internal`, read the two POSTs off a host receiver) is NOT part of this prompt — this container has no Docker socket.
- Do NOT assert a total POST count for the turn. The SDK pushes one POST per task event, so the turn also yields the submitted-task and artifact events; the two status updates are selected by `status.state`.
- Do NOT weaken the absent-token assertion to `Header.Get(...) == ""`. An empty-string token and an absent header must not be conflated — that distinction is the AC's own wording.
- Do NOT measure the turn window against the test's own clock around the HTTP POST. Parse the worker's own `turn start id=<id>` / `turn end id=<id>` lines; a marker that is missing or unparseable fails the spec.
- Do NOT shorten the turn below 30 seconds, and do NOT introduce a knob for the mid-turn delay. The delay is a fixed constant owned by the companion (`midTurnStatusDelay = 10 * time.Second`); the test mirrors it as a constant for its own arithmetic and configures nothing.
- Do NOT add a second `RunSpecs` or a second suite bootstrap. `main_test.go` registers the suite; the new file is another `package main` file in it.
- Do NOT add a `replace` directive to `go.mod` and do not hand-edit `go.mod`. Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command.
- **No credential is logged.** The token in these specs is a test literal; do not print any token in a failure message, and do not write a real bearer token into the test file.
- Tests use Ginkgo v2 / Gomega. `go func()` is permitted in `_test.go`. Assertions belong on the spec goroutine, not inside a `go func()`.
- Do not redeclare a name that `main_internal_test.go` already declares, and do not edit that file to make room for one: both files are in `package main`, so the wire helpers in this file (`a2aEnvelope`, `postA2A`, `captureStderr`, `glogLineTime`, …) must be names no sibling file uses. Read `main_internal_test.go` before adding a package-level identifier.
- Keep every function under the 80-line / 50-statement `funlen` limit and the file well under the repository's 2000-line `revive` limit.
</constraints>

<verification>
Run each of these and confirm the stated result. `make` runs at the repository root; the `ROOTDIR=/workspace` prefix pins that root explicitly rather than leaving it to a `git` probe.

1. `ROOTDIR=/workspace make test` — exits 0, and the new specs appear in the `package main` run (the long-turn spec takes about 35 seconds; that is expected, not a hang).
2. `ROOTDIR=/workspace make precommit` — exits 0.
3. `ls main_a2a_push_internal_test.go` — the file exists.
4. `grep -c 'CanonicalHeaderKey' main_a2a_push_internal_test.go` — prints a count greater than `0` (the absent-token assertion reads the raw header map, so an empty-string token cannot pass it).
5. `grep -c 'turn start id=' main_a2a_push_internal_test.go` — prints a count greater than `0` (the mid-turn window is measured against the worker's own turn-boundary lines).
6. `grep -c 'glogLineTime' main_a2a_push_internal_test.go` — prints a count greater than `0` (the glog header is parsed rather than skipped).
7. `grep -c 'TaskStateWorking' main_a2a_push_internal_test.go` — prints a count greater than `0` (the two status updates are selected by the SDK's own state constants rather than by a hand-typed literal).
8. `grep -c 'blocked address range' main_a2a_push_internal_test.go` — prints a count greater than `0` (the guard-on refusal is asserted by its own reason, not by "some error").
9. `grep -c '5 \* time.Minute' main_test.go` — prints `1` (the suite ceiling was raised for the long-turn spec).
10. `grep -c 'TASK_NOT_FOUND' main_a2a_push_internal_test.go` — prints a count greater than `0` (the unknown-task refusal, which is this file's own half of AC 2).
11. `go build ./...` — succeeds.
</verification>
