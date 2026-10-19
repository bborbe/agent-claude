---
status: completed
spec: [005-a2a-worker-status-push]
summary: Wired the A2A push store and HTTP sender into newInteractiveService via interactive.WithPushNotifications on agent v0.103.0, adding the derived card capability, a fail-closed spec, the worker-side contract docs and a changelog entry
execution_id: agent-claude-exec-020-spec-005-push-store-and-sender
dark-factory-version: v0.198.7
created: "2026-10-17T16:04:54Z"
queued: "2026-10-18T08:34:41Z"
started: "2026-10-18T09:00:55Z"
completed: "2026-10-18T09:13:28Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. BLOCKING COMPANION DEPENDENCY — this prompt cannot compile until
   `github.com/bborbe/agent` ships the push extension point named in the spec's
   Constraints. At `agent@v0.99.1` (the version pinned in this repo today) the
   library's `interactive.NewServiceWithPermissions` takes no push store or sender,
   `interactive.CardConfig` carries only `Name` and `PublicURL`, `interactive/a2a-handler.go`
   builds `a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(&a2aExecutor{cache: s.cache}))`
   with no options, and `interactive/a2a.go` declares `Capabilities: a2a.AgentCapabilities{}`.
   So this prompt's requirement 1 is "adopt the release that carries it", and its
   requirement 2 is written against that release's landed declaration — the prompt
   reads the real signature from the module cache rather than paraphrasing one.
   The spec's own Suggested Decomposition row 1 lists this dependency; its row 2
   (the companion-side mid-turn event) is deliberately NOT generated here, because
   that row's repository is not mounted in this container and a prompt targeting it
   would fail against a read-only module-cache dependency. Row 2 is companion-side
   work in `github.com/bborbe/agent`, hand-implemented or driven by that repo's own
   spec, and the second prompt in this batch (the wire integration test) depends on
   it as well.

2. SETTLED — the store and the sender are constructed inside `newInteractiveService`
   in `main.go`, not behind a new factory in this repository. The spec's Verification
   section pins this (`grep -n 'AllowPrivateNetworks' main.go` must return a line whose
   value is `true`), and AC 10's direct-dependency requirement rests on this module
   importing a2a-go itself. Two module-level calls do not justify a factory package,
   and `docs/dod.md`'s "factory functions are pure composition" rule would then have to
   be applied to a function whose only job is two constructor calls.

3. SETTLED — the fail-closed negative (AC 8) is asserted against a service built through
   the library constructor with the push configuration left unset, not against a new
   opt-out knob in this repository. The spec's Non-goals forbid a per-feature opt-out
   flag, and the capability is meant to follow the wiring rather than be switchable
   independently of it.

4. OUT OF SCOPE, REQUIRED COMPANIONS — (a) the library extension point above;
   (b) the companion's mid-turn status event in `a2aExecutor.Execute` (spec row 2);
   (c) the spawner's receive-and-surface (`bborbe/claude-supervisor`) and the
   controller's registration (`bborbe/a2a-worker-controller`), both hand-implemented
   from the parent task and neither mounted here. This prompt records all four as
   constraints and asks the executor to name them in its final report.
-->

# Wire the A2A push store and sender into the interactive service

<summary>
- The worker image now carries the two pieces the A2A push mechanism needs: a per-task registration store and the HTTP sender that delivers a push.
- The sender is built so a callback on the spawner's own private address is actually delivered rather than refused.
- The worker advertises that it supports push notifications, so a spawner reading its Agent Card learns it may register a callback.
- A worker with push not wired still advertises nothing and still refuses a registration — the capability is never advertised independently of the machinery that delivers it.
- The capability is derived from the wiring rather than configured separately, so the two cannot disagree.
- The A2A SDK becomes a direct dependency of this module, which is what makes the store and the sender this repository's own construction.
- The worker-side contract is documented next to the existing A2A run docs, so it outlives this change.
- The change is recorded in the changelog.
- Nothing else about the service changes: readiness, metrics, prompt intake, the permission endpoint, the Agent Card's other fields, authentication and the task-routed Kafka path are untouched.
- The library change this depends on, the spawner side and the controller side are explicitly not part of this change.
</summary>

<objective>
Build the A2A push-notification store and the HTTP push sender in this repository, with the sender's private-network guard opted out so the spawner's same-host callback is reachable, and pass both into the interactive service that `newInteractiveService` constructs in `main.go`. End state: the Agent Card this image serves advertises `pushNotifications: true`, a registration naming an existing task round-trips while one naming an unknown task is refused, and a service built with the push machinery unset advertises `false` and refuses a registration with `PUSH_NOTIFICATION_NOT_SUPPORTED` — so the capability can never be advertised without the machinery that honours it.
</objective>

<context>
Read `docs/dod.md` — the Definition of Done you are graded against (it requires a `## Unreleased` changelog entry, a README update when usage changes, and forbids `replace` directives in `go.mod`).

**The blocking prerequisite.** The A2A handler and the Agent Card are built inside `github.com/bborbe/agent`'s `interactive` package, and at the version this repository pins today (`v0.99.1`) that package exposes no way for a caller to supply a push store, a sender, or a capability decision:

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/a2a-handler.go
inner := a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(&a2aExecutor{cache: s.cache}))

// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/a2a.go
Capabilities:       a2a.AgentCapabilities{},

// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/service.go
func NewServiceWithPermissions(
	sessions agentlib.SessionFactory,
	listen string,
	providerBaseURL string,
	registry *prometheus.Registry,
	auth Auth,
	card CardConfig,
	permissions PermissionRegistry,
	sessionIdleTimeout time.Duration,
	maxSessions int,
) Service

// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/a2a.go
type CardConfig struct {
	Name      string
	PublicURL string
}
```

The companion release must extend that package to accept the push configuration and to set the card's capability from it (spec Constraints, "Required companion change in `github.com/bborbe/agent`"). **This repository cannot make that change** — it is a dependency. Requirement 1 adopts the release that carries it; requirement 2 writes the call site against that release's own declaration. If no released version carries it, stop and report (requirement 1, step d) rather than writing code that cannot compile.

Files you will read and change (repo-relative):
- `main.go` — the `application` struct, `Run`, `runService`, `newInteractiveService`. `newInteractiveService` is the only function this prompt changes.
- `main_internal_test.go` — `package main` (not `main_test`) because `application` is unexported; the `RunSpecs` in `main_test.go` runs these specs. It already contains `Describe("application.newInteractiveService")`, which serves the real Agent Card route and asserts the advertised name and URL — extend that Describe rather than adding a second one.
- `go.mod` — pins `github.com/bborbe/agent v0.99.1`, and lists `github.com/a2aproject/a2a-go/v2 v2.6.0` as **indirect** today. Do not hand-edit `go.mod`; `go mod tidy` (run by `make precommit`) promotes a2a-go once `main.go` imports it.
- `README.md` — has a `### Service mode` paragraph and an `Env Vars` table.
- `CHANGELOG.md` — the newest section today is `## v0.17.1`; there is no `## Unreleased` section yet.
- `docs/local-docker-run.md` and `docs/cluster-a2a-run.md` — the existing A2A run docs. The new contract note sits beside them (see requirement 5); neither file is otherwise changed.

The SDK surface you will call. These signatures are quoted verbatim from the module cache at `$(go env GOMODCACHE)/github.com/a2aproject/a2a-go/v2@v2.6.0` and are the contract; do not paraphrase them.

```go
// a2asrv/push — import path github.com/a2aproject/a2a-go/v2/a2asrv/push
// The package name is `push`, which COLLIDES with the prometheus push import
// already in main.go (`github.com/prometheus/client_golang/prometheus/push`).
// Import it under an alias, e.g. a2apush.
func NewInMemoryStore() *InMemoryPushConfigStore
func NewHTTPPushSender(config *HTTPSenderConfig) *HTTPPushSender

type HTTPSenderConfig struct {
	// Timeout is used to configure internal [http.Client]. Zero means the
	// sender's own 30-second default.
	Timeout time.Duration
	// FailOnError can be set to true to make push sending errors trigger execution cancelation.
	FailOnError bool
	// AllowPrivateNetworks disables the built-in SSRF guard that rejects webhook URLs
	// resolving to loopback, private, link-local or unspecified addresses.
	AllowPrivateNetworks bool
}
```

`*InMemoryPushConfigStore` satisfies `push.ConfigStore` and `*HTTPPushSender` satisfies `push.Sender`:

```go
// a2asrv/push/api.go
type Sender interface {
	SendPush(ctx context.Context, config *a2a.PushConfig, event a2a.Event) error
}

type ConfigStore interface {
	Save(ctx context.Context, taskID a2a.TaskID, config *a2a.PushConfig) (*a2a.PushConfig, error)
	Get(ctx context.Context, taskID a2a.TaskID, configID string) (*a2a.PushConfig, error)
	List(ctx context.Context, taskID a2a.TaskID) ([]*a2a.PushConfig, error)
	Delete(ctx context.Context, taskID a2a.TaskID, configID string) error
	DeleteAll(ctx context.Context, taskID a2a.TaskID) error
}
```

What the companion library does with them, and what you must NOT do here: it applies `a2asrv.WithPushNotifications(store, sender)` as a `RequestHandlerOption` when it builds the JSON-RPC handler, and sets the card's capability from the same configuration. For reference, the SDK side of that wiring is:

```go
// a2asrv/handler.go — import path github.com/a2aproject/a2a-go/v2/a2asrv
func WithPushNotifications(store push.ConfigStore, sender push.Sender) RequestHandlerOption
func WithCapabilityChecks(capabilities *a2a.AgentCapabilities) RequestHandlerOption

// a2a/agent.go — the field the served card carries
type AgentCapabilities struct {
	// PushNotifications indicates if the agent supports sending push notifications for asynchronous task updates.
	PushNotifications bool `json:"pushNotifications" yaml:"pushNotifications" mapstructure:"pushNotifications"`
	// Streaming indicates if the agent supports streaming responses.
	Streaming bool `json:"streaming" ...`
	// ...
}
```

The SDK's handler refuses a registration when push is not configured, and its error maps to a stable JSON-RPC code — that is what the fail-closed spec in requirement 4 asserts:

```
a2a.ErrPushNotificationNotSupported -> code -32003, reason PUSH_NOTIFICATION_NOT_SUPPORTED
a2a.ErrTaskNotFound                 -> code -32001, reason TASK_NOT_FOUND
```

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-library-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mod-dependency-fix-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/readme-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Adopt the companion release that carries the push extension point.**

   a. List the available releases: `go list -m -versions github.com/bborbe/agent`.

   b. Bump to the newest one and tidy: `go get github.com/bborbe/agent@<version>` then `go mod tidy`.

   c. **Read the landed declaration before writing any call site.** Open, in the module cache at `$(go env GOMODCACHE)/github.com/bborbe/agent@<version>/interactive/`:
      - `service.go` — the `NewServiceWithPermissions` signature and the option seam: does the parameter list end in a trailing variadic `opts ...ServiceOption`, and does `interactive.WithPushNotifications(store push.ConfigStore, sender push.Sender) ServiceOption` exist?
      - `a2a.go` — the `CardConfig` fields (is the push capability a field here, or derived inside from the store and sender?);
      - `a2a-handler.go` — whether the handler is now built with `a2asrv.WithPushNotifications(...)` and a capability check.
      The parameter and field names in requirement 2 are the ones you read there, not the ones sketched below. The sketch fixes *what* must be passed (a `push.ConfigStore` built by `push.NewInMemoryStore()` and a `push.Sender` built by `push.NewHTTPPushSender(...)`); the landed declaration fixes *how* it is spelled.

   d. **If no released version carries the extension point** — the test is the **option**, never the arity: `interactive.WithPushNotifications` does not exist in the package and the constructor's parameter list ends at its current last parameter with no `opts ...ServiceOption` after it. ⚠️ Do **not** conclude "not landed" from `NewServiceWithPermissions` still taking nine arguments — it always will. The frozen seam is a **trailing variadic option**, so the nine base parameters are unchanged *by design* and the store and sender are carried by one `interactive.WithPushNotifications(store, sender)` value appended after them; an arity check reports "not landed" on a release that landed. If the option is genuinely absent, then STOP and report: the companion change must land first, this repository cannot make it, and requirement 2 cannot be written. Do not implement the library here, do not fork it, do not vendor it, and do not add a `replace` directive (requirement 3 and `docs/dod.md` forbid that).

2. **Build the store and the sender in `newInteractiveService` and pass them through.** `main.go`, function `newInteractiveService`. Everything the function does today is preserved; the additions are the two constructions and the single trailing option value that carries both of them into the constructor.

   ```go
   func (a *application) newInteractiveService(
       sessions agentlib.SessionFactory,
       registry *prometheus.Registry,
       permissions interactive.PermissionRegistry,
   ) interactive.Service {
       // The push store and the push sender are built here, in this repository,
       // rather than inside the library: the A2A protocol puts the registration on
       // the agent's own endpoint, so the worker holds its own per-task
       // registrations, and building them here is what makes a2a-go a direct
       // dependency of this module.
       store := a2apush.NewInMemoryStore()
       // AllowPrivateNetworks is mandatory, not a preference. The SDK's sender
       // refuses loopback, private, link-local and unspecified targets by default,
       // and the callback the spawner registers is exactly such an address: a
       // container reaches the Mac host through the Docker gateway, which is
       // private. Without the opt-out every push is refused, the refusal is logged
       // only (the sender's fail-on-error default keeps it from failing the turn),
       // and the channel is silently inert while looking deployed. The opt-out
       // relaxes the resolved-IP range check alone — the redirect cap and the
       // cross-host token stripping still apply and must not be weakened further.
       sender := a2apush.NewHTTPPushSender(&a2apush.HTTPSenderConfig{
           AllowPrivateNetworks: true,
       })
       return interactive.NewServiceWithPermissions(
           sessions,
           a.Listen,
           a.ProviderBaseURL,
           registry,
           interactive.NewAuthToken(a.InteractiveAuthToken),
           interactive.CardConfig{
               Name:      a.A2AAgentName,
               PublicURL: a.A2APublicURL,
               // plus whatever the landed CardConfig carries for the push
               // capability, if it carries one at all
           },
           permissions,
           interactive.DefaultSessionIdleTimeout,
           interactive.DefaultMaxSessions,
           // plus, as the LAST argument, the trailing variadic option that
           // carries both: interactive.WithPushNotifications(store, sender)
       )
   }
   ```

   Notes, each load-bearing:
   - **Leave the sender's `FailOnError` unset.** Its zero value is `false`, which is the spec's invariant: a failing push is logged and swallowed and must never cancel the turn. Do not write the field into the struct literal at all.
   - **Leave `Timeout` unset.** Its zero value selects the sender's own 30-second default, which is the timeout the spec's failure-modes table refers to. Do not add a timeout field to `application`, an env var or a flag.
   - **Import the push package under an alias.** `main.go` already imports `github.com/prometheus/client_golang/prometheus/push` as `push`; a second import with the package name `push` is a compile error. Use `a2apush "github.com/a2aproject/a2a-go/v2/a2asrv/push"`. `goimports-reviser` (run by `make precommit`) orders the import block; do not hand-order it.
   - **Do not add a configurable mid-turn delay, a per-feature opt-out flag, or any new `application` field, env var or CLI flag.** The spec's Non-goals forbid both the delay knob and the opt-out flag, and this change needs neither: the capability follows the wiring.
   - **Do not set the card's capability by hand** if the landed library derives it from the store and sender — read `a2a.go`'s `newAgentCard` / `CardConfig` and follow it. Desired Behavior 1 requires the capability to be set from the same configuration that wires the machinery, and it is the library's job per the spec's Constraints.
   - **Do not touch the task-routed path in `Run`.** A task-routed Kafka job serves no HTTP and gains no push. `runService` needs no change: it already calls `a.newInteractiveService(...)` and passes the result to `run.CancelOnFirstFinish` / `interactiveService.Run`.

3. **Make `github.com/a2aproject/a2a-go/v2` a direct dependency of this module.** Write requirement 2's import first, then let `go mod tidy` (run by `make precommit`, and by `make ensure`) promote the entry. Do not hand-edit `go.mod`, and do not add a `replace` directive.

4. **Add the two specs to `main_internal_test.go`.** Both live in the existing `Describe("application.newInteractiveService")`, in `package main`. Do not create a new test file in this prompt — the wire-level test is the next prompt in this batch.

   a. **The card advertises push support (AC 1).** Extend the existing spec, or add a sibling to it, so it parses the served card into a struct carrying the capability and asserts the field rather than asserting a substring of the body:

      ```go
      var card struct {
          Name         string `json:"name"`
          Capabilities struct {
              PushNotifications bool `json:"pushNotifications"`
          } `json:"capabilities"`
      }
      Expect(json.Unmarshal(recorder.Body.Bytes(), &card)).To(Succeed())
      Expect(card.Capabilities.PushNotifications).To(BeTrue())
      ```

      The service under test is the one `app.newInteractiveService(&agentmocks.SessionFactory{}, prometheus.NewRegistry(), interactive.NewPermissionRegistry())` returns, served through `service.Handler()` on the `/.well-known/agent-card.json` route — the same shape the existing spec uses. A `ContainSubstring("pushNotifications")` assertion does not satisfy this: the field's zero value marshals into the body as `"pushNotifications":false`, so a substring check passes on the unwired worker.

   b. **The worker fails closed (AC 8).** Build a second service through the same library constructor with the push configuration left unset — pass **no** option at all, leaving the trailing variadic empty; do **not** pass a zero-valued store or sender, because the option's fail-loud rule rejects a half-supplied one — and assert both halves:

      - the served card's `capabilities.pushNotifications` is **false**;
      - a `CreateTaskPushNotificationConfig` JSON-RPC call against that service's `/a2a` route is refused, and the response body carries the reason `PUSH_NOTIFICATION_NOT_SUPPORTED` (JSON-RPC code `-32003`). Assert on the reason string or the code, not merely on "an error came back" — a registration naming an unknown task is also refused, with `TASK_NOT_FOUND` (`-32001`), and the two must not be confused.

      The call is a POST to `/a2a` carrying `Content-Type: application/json`, the service's bearer token in an `Authorization: Bearer <token>` header (the route is gated; without the header the gate answers 401 before the handler runs), and a body of:

      ```json
      {"jsonrpc":"2.0","id":1,"method":"CreateTaskPushNotificationConfig","params":{"taskId":"unknown-task","url":"http://127.0.0.1:1/push","token":"tok"}}
      ```

      Decode the response into an anonymous struct declared inside the spec, or a distinctly named package-level type — do **not** declare a generic name such as `a2aEnvelope` or a shared `postA2A` helper here: the next prompt in this batch adds a second `package main` test file that owns the wire helpers, and two files in one package cannot declare the same identifier.

      Omitting the option **is** the way to build a service with the push configuration unset — the frozen seam makes an option-less call site produce a service indistinguishable from today's, which is exactly the state this half asserts. Only if the landed library turns out to make the push configuration mandatory — a required parameter or a constructor that refuses to build without it — report that and skip this half rather than adding an opt-out knob, a second constructor or a config field in this repository. State the gap in your final report so the spec's AC 8 can be re-homed.

      The other half of AC 8's negative — the same registration refused when push *is* wired but the task is unknown, with `TASK_NOT_FOUND` — belongs to the next prompt's receiver-based test, which is where a task can actually be created. Do not try to create a task here: this prompt's specs never run a turn.

5. **Document the worker-side contract.** Add a short subsection to `README.md`, immediately after the existing `### Service mode` paragraph, and a short note in `docs/local-docker-run.md` beside the A2A run instructions there. The text must state the contract as domain knowledge that outlives this change — the capability is advertised on the card; the registration is per task and requires the task to exist; the token travels as the `A2A-Notification-Token` header and nowhere else; the callback must be a private-network address, which is why the sender's guard is opted out; and the mid-turn status is one working-state update per turn. Suggested text for the README subsection (adapt the wording, keep every fact):

   ```markdown
   ### A2A push notifications

   The A2A endpoint accepts a push-notification registration per task, and the Agent Card
   advertises `capabilities.pushNotifications: true` so a caller knows it may register one.
   The registration is created with the `CreateTaskPushNotificationConfig` JSON-RPC method
   (the protocol's legacy name for it is `tasks/pushNotificationConfig/set`) and requires the
   task to already exist: the handler refuses a registration naming a task its task store does
   not hold, so register after the turn's first `SendMessage` has created the task. Every task
   event of a registered task is POSTed to the registered URL as the A2A stream-response
   envelope, and the registered token travels as the `A2A-Notification-Token` header — never in
   the body and never in the log; a config that carries no token sends no header at all. The
   callback must be a private-network address, which is why the sender is built with
   `AllowPrivateNetworks: true`: a container reaches the host through the Docker gateway, and
   the SDK's default guard refuses exactly that range. A failing push is logged and swallowed
   and never fails the turn. The registration route is gated by the same bearer token as every
   other non-exempt route.
   ```

   Do not add env vars (this change adds none) and do not restructure the README.

6. **Add a `## Unreleased` section to `CHANGELOG.md`.** There is none today — the newest section is `## v0.17.1`. Insert `## Unreleased` above it, matching the existing bullet shape (a `- feat:` line with a prose explanation after an em dash). It must state: the worker builds the A2A push-notification store and sender itself, the sender is built with the private-network guard opted out because the spawner's callback is a same-host private address, the Agent Card advertises `capabilities.pushNotifications`, and the capability follows the wiring so a worker without it advertises nothing and refuses a registration. Do not touch any released section. This is the only prompt in the batch that writes a changelog entry.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `AllowPrivateNetworks` is `true` and the sender's `FailOnError` and `Timeout` are left at their zero values, (ii) the store and sender are constructed in `newInteractiveService` and not behind a new package in this repository, (iii) no new `application` field, env var, flag or opt-out knob was added, and (iv) `/readiness`, `/metrics`, `/prompt`, `/permission` and the card's other fields keep their current behavior.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Run `make` at the repository root, and pass `ROOTDIR=/workspace` on every `make` call in this container.
- **The library change is a dependency, not a deliverable.** `github.com/bborbe/agent`'s `interactive` package must be extended to accept the push store and sender and to set the card's capability from them, and the companion's `a2aExecutor.Execute` must emit the mid-turn working-state status. Both are that repository's work — hand-implemented or driven by its own spec. Do NOT implement either here, do NOT fork the library, do NOT vendor it, and do NOT add a `replace` directive (AC 10 and `docs/dod.md` forbid the last).
- **The spawner and the controller are out of scope.** Receiving and surfacing the pushed status is `bborbe/claude-supervisor` work and registering the callback is `bborbe/a2a-worker-controller` work, both hand-implemented from the parent task. Neither checkout is mounted in this container; do not attempt to modify either, and do not reimplement either here.
- The k8s arm is out of scope: a k8s pod cannot reach a Mac-local callback. This change is the docker arm only.
- `AllowPrivateNetworks: true` is mandatory and is the decision that makes the channel work at all. It relaxes only the resolved-IP range check: the redirect cap and the cross-host token stripping stay in force and must not be weakened further.
- `FailOnError` stays at its default (false). A push failure is logged and swallowed; it must not cancel the turn.
- The mid-turn delay is a fixed constant owned by the companion (`midTurnStatusDelay = 10 * time.Second`). Do NOT add an env var, flag, config field or `application` struct field that exposes it, and do NOT add a per-feature opt-out flag that disables the push — the delay is fixed and the channel is always on.
- Do NOT invent a bespoke callback contract, a bespoke token field, or a custom JSON payload shape — the A2A standard path (`CreateTaskPushNotificationConfig` plus the SDK's own sender) is the requirement, not one option among several.
- Do NOT invent a custom handler that bypasses the SDK's task store to accept a pre-turn registration — the store's ownership check is the SDK's.
- The existing surface is unchanged: `/readiness`, `/metrics`, `/prompt`, `/permission`, the A2A JSON-RPC binding, the session cache, per-session locking, the body cap, the authentication gate and the Agent Card's other fields all keep their current behavior and response shapes. The task-routed Kafka path serves no HTTP and gains nothing.
- **No credential is logged.** The registered token, the interactive bearer token and every environment value stay out of the log and out of the pushed payload. Do not add a `glog` line that prints a token, and do not print one in a test failure message.
- Do NOT add a `replace` directive to `go.mod` and do not hand-edit `go.mod` — `go mod tidy` (run by `make precommit`) does the promotion. Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command.
- `cmd/run-task/main.go` is the local file-based CLI mode and is out of scope: it does not serve HTTP, gains no push, and must still compile untouched. This prompt changes no exported signature, so it needs no change.
- Errors use `github.com/bborbe/errors` — `errors.Wrap(ctx, err, "msg")`, `errors.New(ctx, "msg")`, `errors.Errorf(ctx, "fmt", ...)`. Never `fmt.Errorf`, never a bare `return err`.
- Tests use Ginkgo v2 / Gomega, in the existing `package main` suite registered by `main_test.go`. Do not add a second `RunSpecs`.
- Keep every new identifier in `main_internal_test.go` spec-local or distinctly named. The next prompt in this batch adds another file to the same package, so a generic package-level name (`a2aEnvelope`, `postA2A`, `captureStderr`, …) would collide at compile time.
- Keep every function under the 80-line / 50-statement `funlen` limit; `newInteractiveService` must stay well under it.
</constraints>

<verification>
Run each of these and confirm the stated result. `make` runs at the repository root; the `ROOTDIR=/workspace` prefix pins that root explicitly rather than leaving it to a `git` probe.

1. `ROOTDIR=/workspace make test` — exits 0.
2. `ROOTDIR=/workspace make precommit` — exits 0.
3. `grep -n 'AllowPrivateNetworks' main.go` — prints at least one line, and its value is `true`.
4. `! grep -qE '^[[:space:]]*FailOnError:' main.go` — exits 0. The field is left at its zero value; a struct-literal assignment is what this catches, and setting it explicitly would be a change to the spec's invariant rather than a clarification of it.
5. `! grep -qE '^[[:space:]]*Timeout:' main.go` — exits 0 (the sender's own 30-second default is the timeout the spec's failure-modes table refers to; no timeout is configured here).
6. `grep -c 'a2apush.NewInMemoryStore' main.go` — prints `1`.
7. `grep -c 'a2apush.NewHTTPPushSender' main.go` — prints `1`.
8. `grep -n 'a2aproject/a2a-go' go.mod` — prints a require line carrying **no** `// indirect` suffix, and `! grep -q 'a2aproject/a2a-go.*// indirect' go.mod` exits 0 (the dependency is direct).
9. `grep -c 'pushNotifications' main_internal_test.go` — prints a count greater than `0` (the card capability is asserted as a parsed field).
10. `grep -c 'PUSH_NOTIFICATION_NOT_SUPPORTED' main_internal_test.go` — prints a count greater than `0` (the fail-closed negative asserts the SDK's own refusal reason rather than "some error").
11. `grep -n '^## Unreleased' CHANGELOG.md` — prints exactly one line.
12. `grep -c 'A2A-Notification-Token' README.md` — prints a count greater than `0` (the header the token travels in is documented).
13. `! grep -qE '^replace' go.mod` — exits 0 (`docs/dod.md` forbids a `replace` directive).
14. `go build ./...` — succeeds.
</verification>
