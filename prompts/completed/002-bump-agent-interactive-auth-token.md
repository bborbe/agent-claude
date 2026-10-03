---
status: completed
summary: Bumped github.com/bborbe/agent to v0.94.0 and wired the shape-scoped INTERACTIVE_AUTH_TOKEN bearer-token gate into the service agent path
execution_id: agent-claude-auth-exec-002-bump-agent-interactive-auth-token
dark-factory-version: v0.196.0
created: "2026-10-03T09:48:16Z"
queued: "2026-10-03T10:06:51Z"
started: "2026-10-03T10:07:43Z"
completed: "2026-10-03T10:12:32Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SETTLED — the token field is `required:"false"`, not `required:"true"`. `ValidateRequired` in
   github.com/bborbe/argument/v2@v2.13.2 (argument_validate.go) walks every exported field carrying
   `required:"true"` on every parse, with no agent-shape gating. A `required:"true"` tag would
   therefore make INTERACTIVE_AUTH_TOKEN mandatory for EVERY agent-claude process — including the
   task-routed Kafka jobs, which never serve HTTP and have no use for the token — and every
   deployment running those jobs would have to supply the variable or they would refuse to start.
   The contract's guarantee ("a service that cannot authenticate fails to start") is shape-scoped,
   so the enforcement lives in `runService`, where the agent shape is known — exactly the precedent
   the `TaskContent` field already sets. No task-job blast radius.

2. SETTLED — the field uses `display:"length"`, not `display:"password"`. `Print` in
   github.com/bborbe/argument/v2@v2.13.2 (argument_print.go) handles only `hidden` and `length`;
   every other value falls through to the default branch that logs the value. `service.Main` calls
   `argument.ParseAndPrint`, so `display:"password"` would write the bearer token into the startup
   log in plaintext. `display:"length"` is the value the library actually redacts and the one the
   go-k8s-binary-conventions guide mandates for secret fields.

3. OUT OF SCOPE, LIVE BUG — file separately. `main.go`'s existing `AnthropicAuthToken` field carries
   `display:"password"`, which the argument library does not honour, so ANTHROPIC_AUTH_TOKEN is
   already printed verbatim in this binary's startup log. Same root cause as note 2, but it is a
   pre-existing field this prompt must not touch. It deserves its own change.

4. The Kubernetes manifest change that supplies INTERACTIVE_AUTH_TOKEN as a pod secret is out of
   scope for this prompt (it lives in the config repo, not here). It must land with or before the
   image bump, or the service pod will not start.
-->

# Bump github.com/bborbe/agent to v0.94.0 and wire the interactive auth token

<summary>
- The interactive service in the `agent-claude` image now authenticates its gated HTTP routes instead of serving them open to anything that can reach the port.
- The service requires a bearer token and refuses to start without one, so a pod that cannot authenticate never serves unauthenticated.
- The shared agent library is bumped to the release that added the authentication parameter.
- The token is bound through this binary's own configuration struct, so it gets the same validation and log-redaction treatment as every other setting — not read ad hoc from the environment.
- The token is supplied by the deployment as a runtime-only secret and is never written into source, an image layer or a committed manifest.
- The token's value is redacted from the startup configuration log — only its length is printed.
- The task-routed Kafka job path is completely unchanged — it never serves HTTP, so it does not need the token.
- Every existing test still passes, and a new spec covers the service's refusal to start when it has no token.
- The change is recorded in the changelog and the environment-variable table.
- Adding the Kubernetes manifest that supplies the secret is explicitly not part of this change.
</summary>

<objective>
Give the interactive service this binary serves the authentication the shared `github.com/bborbe/agent` library now requires, by bumping the library to the release that added the `auth` parameter and binding the bearer token through this binary's own argument struct. End state: a service pod authenticates every gated route with `Authorization: Bearer <token>`, and a service pod that has no token fails to start instead of serving its prompt-intake and permission routes unauthenticated. The task-routed job path must behave exactly as it does today.
</objective>

<context>
This repository has no root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md` (the Definition of Done you are graded against; it forbids `replace` directives in `go.mod` and requires a `## Unreleased` changelog entry).

Files you will read and change (repo-relative):
- `main.go` — the `application` argument struct, `Run`, and `runService`. Read all three before editing.
- `go.mod` — the current pin is `github.com/bborbe/agent v0.93.1` (direct require block).
- `main_internal_test.go` — the argument-parsing specs and the `runService` spec. This is `package main` (not `main_test`) because `application` is unexported; the `RunSpecs` in `main_test.go` runs these specs.
- `CHANGELOG.md` — the newest section today is `## v0.5.0`; there is no `## Unreleased` section yet.
- `README.md` — has a "Service mode" paragraph and an "Env Vars" table.

The library API you must call — read the landed source before you write anything. After the bump it lives in the Go module cache; these signatures are quoted from the landed files and are the contract. Do not paraphrase them.

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.94.0/interactive/service.go
// package interactive — import path github.com/bborbe/agent/interactive (already imported as `interactive` in main.go)

// auth is the fifth parameter, before permissions.
func NewService(
	sessions agentlib.SessionFactory,
	listen string,
	providerBaseURL string,
	registry *prometheus.Registry,
	auth Auth,
) Service

func NewServiceWithPermissions(
	sessions agentlib.SessionFactory,
	listen string,
	providerBaseURL string,
	registry *prometheus.Registry,
	auth Auth,
	permissions PermissionRegistry,
) Service
```

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.94.0/interactive/auth.go

// NewAuthToken returns the Auth that requires every gated request to present token as
// its bearer credential. An empty token is not a usable credential: it yields the same
// fail-closed value as the zero Auth.
func NewAuthToken(token string) Auth

// AuthFromEnv reads the bearer token from the process environment and returns the Auth
// that requires it. It returns an error when the variable is unset or empty.
func AuthFromEnv(ctx context.Context) (Auth, error)

// AuthDisabled is the explicit opt-out: a service built with it serves every route
// without authentication.
var AuthDisabled = Auth{disabled: true}
```

The upstream contract these signatures belong to is NOT in this repository (it lives in `bborbe/agent`). The parts that matter here, restated:
- Every route except `/readiness` and `/metrics` requires `Authorization: Bearer <token>`; a missing, malformed or wrong credential is refused with `401` before the route's own handler runs.
- The token is read at startup and exists only as a runtime-injected value: never in source, never in an image layer, never in a committed manifest.
- An unset or empty token is an error, so a service that cannot authenticate fails to start rather than serving unauthenticated — the pod does not reach Ready.
- The token is never logged, at any verbosity, and never returned in an error.

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-build-args-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-k8s-binary-conventions.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Bump `github.com/bborbe/agent` from `v0.93.1` to `v0.94.0`.** The direct require block in `go.mod` currently reads `github.com/bborbe/agent v0.93.1`. Run:

   ```bash
   go get github.com/bborbe/agent@v0.94.0
   ```

   Do not run `go mod vendor` and do not write `-mod=vendor` anywhere.

   **STOP CONDITION.** `v0.94.0` must resolve. If `go get` cannot resolve it (tag absent, or the module proxy is unreachable), **stop, change nothing further, and report the blocker** naming the version you tried to resolve and the newest version that did resolve. Do NOT add a `replace` directive and do NOT fall back to a pseudo-version — `docs/dod.md` forbids `replace` (it breaks remote install).

2. **Verify the new API is actually present in the resolved version — before writing any code.** Run:

   ```bash
   sed -n '/^func NewServiceWithPermissions(/,/^) Service {/p' \
     "$(go env GOMODCACHE)/github.com/bborbe/agent@$(go list -m -f '{{.Version}}' github.com/bborbe/agent)/interactive/service.go"
   ```

   The printed signature must list `auth Auth,` as the fifth parameter, immediately before `permissions PermissionRegistry,`. If it does not — or if the file does not exist — the resolved version does not contain this API: stop and report, do not improvise around it.

3. **Add the bearer-token field to the `application` struct in `main.go`.** Put it in the service-mode group next to `Listen` and `ProviderBaseURL`, and copy the tag shape of the `AnthropicAuthToken` field earlier in the same struct (`Listen`/`ProviderBaseURL` are the placement anchors; `AnthropicAuthToken` is only the tag-shape model):

   ```go
   // InteractiveAuthToken is the bearer credential the interactive service requires on
   // every gated route. It is delivered as a runtime-only pod secret and is never
   // written into source, an image layer or a committed manifest. The requirement is
   // enforced in runService, where the agent shape is known: a service that cannot
   // authenticate must fail to start rather than serve unauthenticated.
   InteractiveAuthToken string `required:"false" arg:"interactive-auth-token" env:"INTERACTIVE_AUTH_TOKEN" usage:"Bearer token required by the interactive service's gated routes" display:"length"`
   ```

   Notes on the tags — each is load-bearing:
   - `env:"INTERACTIVE_AUTH_TOKEN"` is fixed. It is the name the library reads and the name the upstream contract documents. Do not invent a different name and do not rename it.
   - `required:"false"` is deliberate, not an oversight. `argument.Parse` evaluates a `required:"true"` tag for every agent shape, so making the field required at parse time would also reject the task-routed jobs, which never serve HTTP and have no use for the token. The requirement is shape-scoped and is enforced in `runService` instead (requirement 4). Do not change this to `required:"true"`.
   - `display:"length"` is the redaction mode. Do NOT use `display:"password"` here: the argument library only honours `display:"hidden"` and `display:"length"`, and a `display:"password"` value is printed verbatim by the startup configuration log — which would leak the bearer token. (`display:"length"` prints only the length.)
   - Do not add a `default` tag. A default would defeat the requirement.

4. **Enforce the token in `runService`, then pass it into the service constructor.** Two edits, both inside `runService`.

   First, add the shape-scoped guard as the first statement of the function body, before anything is constructed or bound:

   ```go
   if a.InteractiveAuthToken == "" {
       return errors.Errorf(
           ctx,
           "INTERACTIVE_AUTH_TOKEN is required for a service agent; a service that cannot authenticate must not serve unauthenticated",
       )
   }
   ```

   The guard belongs here rather than in a `required:"true"` struct tag: `argument.Parse` evaluates required tags for every agent shape, so a tag would also reject the task-routed jobs, which never serve HTTP. `runService` is reached only when `a.AgentType == factory.AgentTypeService`, so it is where the agent shape is known — the same pattern the `TASK_CONTENT` check in `Run` already uses. `github.com/bborbe/errors` is already imported in `main.go`.

   Second, pass the token to the constructor. The current call site is:

   ```go
   return interactive.NewServiceWithPermissions(
       sessions,
       a.Listen,
       a.ProviderBaseURL,
       registry,
       permissions,
   ).Run(ctx)
   ```

   Insert the auth argument between `registry,` and `permissions,` so it is the fifth parameter:

   ```go
   return interactive.NewServiceWithPermissions(
       sessions,
       a.Listen,
       a.ProviderBaseURL,
       registry,
       interactive.NewAuthToken(a.InteractiveAuthToken),
       permissions,
   ).Run(ctx)
   ```

   `interactive.NewAuthToken` returns the `Auth` the constructor expects. The two arguments are distinct types (`Auth` and `PermissionRegistry`), so a wrong order is a compile error — but pass them in the order shown anyway. Nothing else in `runService` changes: the permission registry, the session factory and the `glog` line stay as they are. The `glog` line must not be extended to print the token.

5. **Update the specs in `main_internal_test.go`.** All specs live in the one `package main` suite.

   a. The two argument-parsing specs (`Context("with a service-agent environment")` and `Context("with a task-agent environment")`) need **no change**. The field is `required:"false"`, so `argument.Parse` succeeds whether or not `INTERACTIVE_AUTH_TOKEN` is set, and both specs must still pass exactly as they are. Do not add the variable to their `BeforeEach` blocks.

   b. Add a new spec inside `Describe("application.runService")` that drives the fail-to-start guarantee directly: construct the application with no token and assert that `runService` returns an error rather than serving.

   ```go
   It("fails to start when no interactive auth token is configured", func() {
       app := &application{Listen: "127.0.0.1:0"}
       err := app.runService(context.Background(), prometheus.NewRegistry(), map[string]string{})
       Expect(err).To(HaveOccurred())
   })
   ```

   Add a comment saying this is the regression guard for the shape-scoped enforcement: the check lives in `runService` (where the agent shape is known) rather than in a `required:"true"` struct tag, because the tag is evaluated for every agent shape and would also reject task-routed jobs. This is the boundary the new code crosses — a struct-equality or constant-value assertion would not exercise it.

   c. In the existing `Describe("application.runService")` "serves until the context is cancelled" spec, add a non-empty token to the constructed value so the spec exercises the real wiring rather than the empty-token failure path:

   ```go
   app := &application{Listen: "127.0.0.1:0", InteractiveAuthToken: "test-token"}
   ```

   Leave the rest of that spec (the `Consistently` / `cancel` / `Eventually` assertions) exactly as it is.

   d. Add one binding spec to `Describe("application argument parsing")` proving the env var reaches the struct field — the boundary this field crosses. Do not touch the two existing Contexts:

      ```go
      It("binds INTERACTIVE_AUTH_TOKEN into the struct field", func() {
          Expect(os.Setenv("INTERACTIVE_AUTH_TOKEN", "test-token")).To(Succeed())
          DeferCleanup(func() { _ = os.Unsetenv("INTERACTIVE_AUTH_TOKEN") })
          app := &application{}
          Expect(argument.Parse(ctx, app)).To(Succeed())
          Expect(app.InteractiveAuthToken).To(Equal("test-token"))
      })
      ```

      This is the contract test for the env-name boundary: a `grep` on the tag proves the string was typed, this proves the library binds it. Without it, a mistyped tag produces a pod that starts, fails the `runService` guard and CrashLoops, with only a grep standing between that and production.

   Do not create new test files. `main_test.go` and `cmd/run-task/main_test.go` contain only the Ginkgo suite bootstrap and need no change.

6. **Add a `## Unreleased` section to `CHANGELOG.md`.** There is none today — the newest section is `## v0.5.0`. Insert `## Unreleased` above it, matching the existing bullet shape (a bold-free `- feat:` / `- chore:` line with a prose explanation after an em dash):

   ```
   ## Unreleased

   - feat: the interactive service now requires a bearer token on every gated route — a service agent refuses to start without `INTERACTIVE_AUTH_TOKEN` instead of serving its prompt-intake and permission routes unauthenticated; the task-routed job path is unchanged and does not need the token; the token is delivered as a runtime-only pod secret and its value is redacted from the startup config log
   - chore: update github.com/bborbe/agent to v0.94.0 — the library adds the required `auth` parameter to `interactive.NewService` and `interactive.NewServiceWithPermissions` and the bearer-token gate that consumes it
   ```

   Do not touch any released section.

7. **Update the `README.md` env-var table and the service-mode paragraph.** Add a row to the "Env Vars" table, using the same conditional-Required idiom the `TASK_CONTENT` row already uses:

   ```
   | `INTERACTIVE_AUTH_TOKEN` | yes (when `AGENT_TYPE=service`) | — | Bearer token the interactive service requires on its gated routes; redacted from the startup log |
   ```

   In the "Service mode" paragraph, add one sentence: every route except readiness and metrics requires `Authorization: Bearer <token>`, and a service pod with no token fails to start. Keep the edit to those two places; do not restructure the README.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) no file in this repository defines or imports a local copy of the `interactive` package, (ii) `main.go` contains neither `AuthFromEnv` nor `AuthDisabled`, (iii) `cmd/run-task/main.go` is untouched and still compiles, and (iv) the token is not required at argument-parse time, so a task-routed job still starts without it.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- The task-routed job path's behaviour must not change: same env contract, same Kafka delivery, same result printing, same exit codes. It never serves HTTP, so it must not gain a requirement for `INTERACTIVE_AUTH_TOKEN`.
- Do NOT edit anything under a local `interactive/` directory — that package lives in the upstream `github.com/bborbe/agent` module, not in this repository. There is no local copy; do not create one, and do not re-implement the auth gate here.
- Do NOT use `interactive.AuthFromEnv`. It reads the process environment directly and would bypass this binary's binding path, which is what gives the value its log redaction. Bind the token as the `application` struct field instead (requirement 3).
- Do NOT use `interactive.AuthDisabled`. It is the explicit opt-out and serves every gated route unauthenticated — the opposite of what this change is for.
- Do NOT change the env var name. `INTERACTIVE_AUTH_TOKEN` is the name the library reads and the name the upstream contract documents.
- Do NOT change the `AnthropicAuthToken` field's tags and do NOT change any other existing field. Only the new field is in scope.
- Do NOT add a `replace` directive to `go.mod` — `docs/dod.md` forbids it (it breaks remote install). If `v0.94.0` does not resolve, stop and report (requirement 1).
- Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command. `vendor/` is a build-time artifact, gitignored, and wiped by `make precommit`.
- Do NOT add the Kubernetes manifest / Config CR change that supplies the secret — it lives in the config repo and is explicitly out of scope. Note in your final report that it must land with or before the image bump.
- `cmd/run-task/main.go` is the local file-based CLI mode and is out of scope: it does not import the `interactive` package, so it must still compile untouched.
- Errors use `github.com/bborbe/errors` — `errors.Wrap(ctx, err, "msg")`, `errors.New(ctx, "msg")`, `errors.Errorf(ctx, "fmt", ...)`. Never `fmt.Errorf`, never a bare `return err`.
- Exported types, functions and constants carry doc comments. Interface → Constructor → Struct → Method.
- Tests use Ginkgo v2 / Gomega. Counterfeiter mocks, if any are needed, are generated via the explicit `//go:generate` line already present in `main_test.go` — do not add a second one.
- Run `make precommit` at the repository root. This repo has a single root Makefile chain; there is no per-service Makefile for the root package.
</constraints>

<verification>
Run each of these and confirm the stated result. `make precommit` runs at the repository root; the `ROOTDIR=/workspace` prefix pins that root path explicitly, because this repo's `Makefile.variables` otherwise resolves `ROOTDIR` from a repository-root probe that is not reliable under the execution container.

1. `ROOTDIR=/workspace make precommit` — exits 0.
2. `grep -n 'github.com/bborbe/agent v0.94.0' go.mod` — prints a line.
3. `grep -n 'env:"INTERACTIVE_AUTH_TOKEN"' main.go` — prints the new field's line, and that line contains `required:"false"` and `display:"length"`.
4. `grep -n 'a.InteractiveAuthToken == ""' main.go` — prints a line (the shape-scoped guard in `runService` exists).
5. `grep -c 'interactive.NewAuthToken(a.InteractiveAuthToken)' main.go` — prints `1`.
6. `! grep -q 'AuthFromEnv' main.go` — exits 0 (the env-reading helper is not used).
7. `! grep -q 'AuthDisabled' main.go` — exits 0 (the opt-out is not used).
8. `grep -c 'InteractiveAuthToken' main_internal_test.go` — prints a count greater than `0` (the token is exercised in the `runService` specs, both the empty-token failure case and the wired case).
9. `grep -n '^## Unreleased' CHANGELOG.md` — prints exactly one line.
</verification>
