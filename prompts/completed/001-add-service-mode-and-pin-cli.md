---
status: completed
summary: Added AGENT_TYPE=service mode to agent-claude (serving the shared interactive readiness/metrics/prompt-intake surface on LISTEN via bborbe/agent v0.92.0), made TASK_CONTENT/TASK_ID optional for service agents, and pinned @anthropic-ai/claude-code@2.1.286 in the Dockerfile
execution_id: agent-claude-exec-001-add-service-mode-and-pin-cli
dark-factory-version: v0.196.0
created: "2026-10-01T08:40:51Z"
queued: "2026-10-01T10:02:16Z"
started: "2026-10-01T10:03:01Z"
completed: "2026-10-01T10:14:41Z"
---

# Add service mode to agent-claude and pin the Claude Code CLI

<summary>
- The `agent-claude` image can now run as a long-lived interactive service, not only as a one-shot Kafka job.
- Service mode is selected by the same `AGENT_TYPE=service` stamp the executor already sets for a service Config — a job Config is unaffected.
- The interactive HTTP surface (readiness, metrics, prompt intake) is provided entirely by the shared library; this repo adds no routing of its own.
- A conversation survives across requests: two prompts on one session id reach the same held Claude process.
- The task-routed job path keeps its current behaviour, its current env contract and its current exit.
- A service pod no longer needs task content or a task id to start, so it stops failing argument parsing before it can serve.
- The Claude Code CLI version baked into the image is pinned instead of floating at image-build time.
- The pin is recorded, so the version the streaming protocol was exercised against is discoverable later.
- Adding the `claude-interactive` Config and its Kubernetes manifests is explicitly not part of this change.
</summary>

<objective>
Let the `agent-claude` image serve the shared interactive agent service as well as its existing one-shot Kafka job, so an interactive Claude worker can hold a conversation across requests — and pin the Claude Code CLI in the image so the stream-json protocol the session implementation speaks cannot change under it at build time. The job path must be unchanged in behaviour; this is an addition, not a replacement.
</objective>

<context>
Read `CLAUDE.md` (repo conventions, the "never commit" rule) and `docs/dod.md` (the Definition of Done you are graded against — it also forbids `replace` directives in `go.mod`).

Files you will change:
- `main.go` — the Kafka job entry point. Read `application`, its struct tags, and the whole `Run` method before editing.
- `pkg/factory/factory.go` — the composition layer. Read `CreateClaudeRunner` as the exemplar for a new sibling factory.
- `Dockerfile` — the image build; the CLI install line is the one with `@anthropic-ai/claude-code` and no version specifier.
- `CHANGELOG.md` — the newest section today is `## v0.2.6`; there is no `## Unreleased` section yet.
- `README.md` — has an "Env Vars" table and a "How It Works" section.

The library API you must call — read the landed source before you write anything; it lives in the Go module cache after the dependency bump, under `$(go env GOMODCACHE)/github.com/bborbe/agent@<version>/`. These signatures are quoted from the landed files and are the contract; do not paraphrase them.

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@<version>/interactive/service.go
// package interactive — import path github.com/bborbe/agent/interactive
type Service interface {
	Handler() http.Handler
	Run(ctx context.Context) error
}

func NewService(
	sessions agentlib.SessionFactory,
	listen string,
	providerBaseURL string,
	registry *prometheus.Registry,
) Service
```

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@<version>/claude/claude-session.go
// package claude — import path github.com/bborbe/agent/claude (already imported as claudelib)
func NewSessionFactory(base ClaudeRunnerConfig, decider PermissionDecider) agentlib.SessionFactory
```

`ClaudeRunnerConfig` lives in `claude/claude-runner-config.go`. Read it yourself and use the fields it declares — do not assume the list from memory. The fields this change populates are `ClaudeConfigDir`, `AllowedTools`, `Model`, `WorkingDirectory` and `Env`; the existing `CreateClaudeRunner` in `pkg/factory/factory.go` sets those same fields, so treat it as the exemplar.

`agentlib.SessionFactory` is the interface declared in `agent_session.go` in the library root package; `claude.NewSessionFactory` returns it and `interactive.NewService` accepts it, so no adapter is needed.

The upstream specs that motivate this change live in the sibling repo `bborbe/agent` (`specs/in-progress/054-shared-interactive-agent-service.md`, `specs/in-progress/055-claude-streaming-session.md`). They are NOT reachable from the container — the contract you need is restated in this prompt. Do not try to read them.

Coding guides (in-container paths, read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-service-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Bump `github.com/bborbe/agent` to a version containing the new API.** `go.mod` currently requires `v0.89.0`. Run `go get github.com/bborbe/agent@latest` then `go mod tidy`.

   Then verify the API is actually present in the resolved version — do this before writing any code:
   ```bash
   ls "$(go env GOMODCACHE)/github.com/bborbe/agent@$(go list -m -f '{{.Version}}' github.com/bborbe/agent)/interactive/"
   ```
   The directory must exist and contain `service.go` declaring `func NewService(`. If it does not, the newest tag predates the API.

   **STOP CONDITION.** The API landed on the library's `master` in three commits after its newest release tag `v0.90.4`: `5c8e359` (session interface + interactive service), `8cfbcbb` (pi session), `8554522` (Claude streaming session). If the resolved version does not contain `interactive/service.go` and `claude/claude-session.go` with the signatures above, then no released version contains the API yet: **stop, change nothing further, and report the blocker** naming those three commits and the newest tag you resolved. Do NOT add a `replace` directive — `docs/dod.md` forbids it, and a `replace` pointing at a local path is exactly the failure this stop condition exists to prevent.

   Ordering note: add the import (step 6) before the final `go mod tidy`, so `go mod tidy` promotes the dependency rather than dropping it.

2. **Add a service-agent discriminator to `application` in `main.go`.** The executor stamps `AGENT_TYPE` from the Config's `spec.type` (`service` for a long-lived identity agent); the binary reads it and never writes it. Add the field next to the existing `Phase` field:

   ```go
   // AgentType is the agent shape stamped by the executor from the Config's
   // spec.type: "service" for a long-running identity agent, empty for a
   // task-routed one. See factory.AgentTypeService.
   AgentType string `required:"false" arg:"agent-type" env:"AGENT_TYPE" usage:"Agent shape: 'service' for a long-running identity agent; empty for a task-routed one"`
   ```

   No `default` tag: an empty value must keep meaning "task-routed", which is what every existing job Config produces.

3. **Add the two service-mode settings to `application`.**

   ```go
   // Listen is the address a service agent binds for readiness, metrics and
   // prompt intake. A task-routed agent runs one task and exits, so it never serves.
   Listen string `required:"false" arg:"listen" env:"LISTEN" usage:"Address for readiness/metrics (service agents only)" default:":9090"`

   // ProviderBaseURL is the provider endpoint a service agent dials for its
   // readiness check. Empty means the check is skipped and reported as skipped
   // rather than guessed at.
   ProviderBaseURL string `required:"false" arg:"provider-base-url" env:"PROVIDER_BASE_URL" usage:"Provider endpoint; a service agent dials it for readiness"`
   ```

   `:9090` is the port the executor's readiness probe targets; do not choose a different default.

4. **Relax `TaskContent` so a service agent can start.** It is currently `required:"true"`. The executor's service reconciler stamps no `TASK_CONTENT` for a service Config, so the framework's required-field validation would reject the pod at argument parsing and it would never reach the service branch. Change the tag to `required:"false"` and extend the usage string to say it is required unless `AGENT_TYPE=service`. The requirement moves into `Run` (step 6), where the agent shape is known.

5. **Make `TaskID` a plain `string`.** It is currently `agentlib.TaskIdentifier`. That type implements `Validate(ctx context.Context) error` and rejects an empty value; `github.com/bborbe/argument/v2`'s `ValidateHasValidation` runs that method on every exported field regardless of its `required` tag, so a service agent — which has no task id — would fail argument parsing for a reason unrelated to its own shape. Change the field type to `string` and convert at the single point of use, inside `Run`'s Kafka deliverer branch: pass `agentlib.TaskIdentifier(a.TaskID)` to `factory.CreateKafkaResultDeliverer`. The existing `if a.TaskID != ""` guard keeps working unchanged.

6. **Add the service-mode branch to `Run`.** Insert it immediately after the block that finishes building `claudeEnv` (the block ending with the `if a.AnthropicModel != "" { claudeEnv["ANTHROPIC_MODEL"] = a.AnthropicModel.String() }` statement) and immediately before the `provider := factory.CreateAgentProvider(` statement:

   ```go
   if a.AgentType == factory.AgentTypeService {
       return a.runService(ctx, registry, claudeEnv)
   }
   if a.TaskContent == "" {
       jobMetrics.RecordRun(agentlib.AgentStatusFailed)
       jobMetrics.RecordDuration(time.Since(start))
       return errors.Errorf(
           ctx,
           "TASK_CONTENT is required for a task-routed agent; it is optional only when AGENT_TYPE=%s",
           factory.AgentTypeService,
       )
   }
   ```

   Nothing above the insertion point changes. The deliverer built earlier is a no-op for a service agent because a service agent carries no `TASK_ID`, which is why the branch can sit below it. Everything below the insertion point — the provider, the agent, the run, the result — is task-only and is skipped in service mode.

   Add the import at the same time: `interactive "github.com/bborbe/agent/interactive"`.

7. **Add `runService` to `main.go`.** This is the long-running half of the binary. Put it after `Run`:

   ```go
   // runService is the long-running half of this binary. A service agent has no
   // task to run, so instead of executing one and exiting it stays alive and
   // answers when addressed, holding one conversation per session id.
   func (a *application) runService(
       ctx context.Context,
       registry *prometheus.Registry,
       claudeEnv map[string]string,
   ) error {
       glog.V(2).Infof(
           "agent-claude service mode: serving readiness, metrics and prompt intake on %s",
           a.Listen,
       )
       sessions := factory.CreateClaudeSessionFactory(
           a.ClaudeConfigDir,
           a.AgentDir,
           claudelib.ParseAllowedTools(a.AllowedToolsRaw),
           a.AnthropicModel,
           claudeEnv,
       )
       return interactive.NewService(sessions, a.Listen, a.ProviderBaseURL, registry).Run(ctx)
   }
   ```

   `interactive.NewService(...).Run(ctx)` blocks until the context is cancelled and returns nil on a graceful shutdown, so `Run` returning it directly is correct — do not wrap it in an extra goroutine, a wait group, or a `service.Run` call.

8. **Add `factory.AgentTypeService` and `factory.CreateClaudeSessionFactory` to `pkg/factory/factory.go`.** Both are pure composition, matching the file's existing `Create*` convention.

   ```go
   // AgentTypeService is the AGENT_TYPE value the executor stamps for a Config whose
   // spec.type is service. The executor owns this value; this side only reads it.
   // The coupling is silent when it breaks: a mismatch means the binary stays in
   // task mode and never serves, with nothing logged as an error.
   const AgentTypeService = "service"
   ```

   `CreateClaudeSessionFactory` takes the same five parameters as `CreateClaudeRunner` and returns `agentlib.SessionFactory` (the root-package interface that `claude.NewSessionFactory` returns — there is no `SessionFactory` type in the `claude` package); it builds a `claudelib.ClaudeRunnerConfig` with those five values and passes it to `claudelib.NewSessionFactory`, whose second argument (the `PermissionDecider`) is `nil`. Pass `nil` deliberately and say so in the doc comment: the library currently ships no production `PermissionDecider` implementation — only the counterfeiter fake in its `mocks/` package — so a nil decider is the only available wiring. A nil decider means the session process is spawned without `--permission-prompt-tool stdio`, and a tool invocation outside the configured allowlist fails that turn loudly instead of blocking on a decider nobody answers. Do not invent a decider, and do not add a field to `ClaudeRunnerConfig` for it.

9. **Pin `@anthropic-ai/claude-code` in `Dockerfile`.** The install line currently reads `&& npm install -g --omit=dev --no-optional @anthropic-ai/claude-code \` with no version specifier, so the version floats at image-build time. Add an explicit `@x.y.z` specifier.

   Choose the version like this, in order:
   a. If this repo records a version for `@anthropic-ai/claude-code` anywhere (`docs/`, `CHANGELOG.md`, a comment), use that one — it is the version the streaming-input work was exercised against.
   b. Otherwise, determine the current stable from the registry: `npm view @anthropic-ai/claude-code version`. Pin that exact version.
   c. If neither is available, stop and report the pin as the blocker rather than inventing a number.

   Add a short comment above the install line naming the chosen version and where the number came from. Do NOT copy the version from a sibling repo's Dockerfile — `agent-pi` pins `@earendil-works/pi-coding-agent@0.87.1`, which is a different package, and that number is wrong here.

10. **Add tests that traverse the boundaries this change crosses.** Two new specs, both cheap:

    a. New file `main_internal_test.go`, `package main` (not `main_test` — the `application` struct is unexported and an external test cannot reach it; Ginkgo registers specs in one global suite per test binary, so the `RunSpecs` already in `main_test.go` runs them). Open the file with a comment saying exactly that, the way the sibling repos do.

    Add a spec that drives the real argument-parsing boundary — `argument.Parse` from `github.com/bborbe/argument/v2`, which runs the same `ParseOnly -> ValidateRequired -> ValidateHasValidation` chain `service.Main` runs via `argument.ParseAndPrint` — and asserts:
    - a **service-agent environment** (`AGENT_TYPE=service`, no `TASK_CONTENT`, no `TASK_ID`) parses with no error, `app.AgentType == "service"`, and `app.Listen == ":9090"`;
    - a **task-agent environment** (`TASK_CONTENT` set, `AGENT_TYPE` unset) parses with no error and leaves `app.AgentType == ""`.

    The first case is the regression guard for steps 4 and 5: before them it fails with `validate required failed` on `TASK_CONTENT` and `validate failed` on `TaskID`, which is precisely how a service pod would have died before serving anything. A struct-equality test on the constant alone would not catch it.

    `argument.Parse` registers flags on the global `flag.CommandLine`, so calling it twice in one process panics with `flag redefined` unless the flag set is reset first. Reset it in `BeforeEach` exactly as `github.com/bborbe/argument/v2`'s own `argument_parse_test.go` does:
    ```go
    flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
    os.Args = []string{"agent-claude"}
    ```
    Set and restore the env vars with `os.Setenv` / `DeferCleanup(func() { _ = os.Unsetenv(...) })`.

    b. In `pkg/factory/factory_test.go`, add a spec asserting `factory.AgentTypeService` equals `"service"`, with a comment that the executor owns that value (`AgentTypeService` in the executor's CRD types package) and that a mismatch is silent — the binary would simply never leave task mode. This pins the cross-repo contract that has no compile-time check.

11. **Update `README.md` and `CHANGELOG.md`.** Add the three new env vars (`AGENT_TYPE`, `LISTEN`, `PROVIDER_BASE_URL`) to the README's "Env Vars" table, change the `TASK_CONTENT` row's Required column from `yes` to `yes (unless AGENT_TYPE=service)` (requirement 4 makes it conditional), and add a short paragraph to "How It Works" describing the second shape: with `AGENT_TYPE=service` the binary serves readiness, metrics and prompt intake on `LISTEN` instead of running one task and exiting. Create a `## Unreleased` section at the top of `CHANGELOG.md` (none exists today) with entries for the service mode, the dependency bump, and the pinned CLI version — name the pinned version in that entry.

**Self-check before finishing:** re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) no `.go` file in this repo imports `net/http`, (ii) the task path's env contract is unchanged apart from `TaskContent` no longer being required at parse time, and (iii) `ClaudeRunnerConfig` gained no field.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- The task-routed job path's behaviour must not change: same env contract, same Kafka delivery, same result printing, same exit codes.
- `cmd/run-task/main.go` is the local file-based CLI mode and is out of scope: it gains no service mode and needs no change. It calls `factory.CreateAgent` and `factory.CreateFileResultDeliverer`, both of which keep their signatures, so it must still compile untouched.
- Do NOT add a `replace` directive to `go.mod` — `docs/dod.md` forbids it (it breaks remote install). If the needed library version is not tagged, stop and report instead (requirement 1).
- Do NOT add any HTTP routing to this repo. No `net/http` import, no `http.ServeMux`, no `http.Handler`, no `ListenAndServe` in this repository's Go sources — the shared library owns the entire HTTP surface (`/readiness`, `/metrics`, `POST /prompt`), and a second copy here is the duplication this change exists to avoid.
- Do NOT add the `claude-interactive` Config CR or its Kubernetes manifests — explicitly out of scope; a separate task owns them.
- Do NOT add a field to `ClaudeRunnerConfig`, and do not change any existing signature in `github.com/bborbe/agent` — the library is consumed by other binaries.
- Do NOT copy `0.87.1` (or any version) from a sibling repo's Dockerfile — that number belongs to a different npm package.
- Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command. `vendor/` is a build-time artifact, gitignored, and wiped by `make precommit`.
- Errors use `github.com/bborbe/errors` — `errors.Wrap(ctx, err, "msg")`, `errors.New(ctx, "msg")`, `errors.Errorf(ctx, "fmt", ...)`. Never `fmt.Errorf`, never a bare `return err`.
- Exported types, functions and constants carry doc comments. Interface → Constructor → Struct → Method. Factory functions are pure composition — no conditionals, no I/O, no `context.Background()`.
- Tests use Ginkgo v2 / Gomega.
</constraints>

<verification>
Run each of these and confirm the stated result.

1. `make precommit` — exits 0.
2. `grep -n '@anthropic-ai/claude-code@' Dockerfile` — prints at least one line (the version specifier is present).
3. `! grep -rqE 'ListenAndServe|ServeMux|http\.Handler' --include='*.go' .` — exits 0, i.e. the grep finds nothing anywhere in this repo. All routing comes from the library.
4. `go build ./...` — succeeds.
</verification>
