---
status: draft
created: "2026-11-10T18:20:00Z"
---

# Adopt CLAUDE_SESSION_ID from the pod environment

<summary>
- The agent reads a session id from its own environment and hands it to the Claude runner, so the CLI runs under the id the executor already recorded for the run.
- Unset means unchanged: a local `run-task` invocation and every existing deployment keep today's behaviour exactly.
- The value reaches both runner shapes; service mode keeps a per-conversation identity only if the library's session factory honours the id it is given (step 0 verifies this).
- No new required configuration: an absent value is not an error at startup, and a malformed one is rejected before the CLI is spawned.
- This is the consumer half of the executor's session-id injection; the library must ship the matching field first (step 0).
</summary>

<objective>
Wire the executor-supplied session id environment variable into the Claude runner's configuration for both the one-shot runner and the session factory, so a spawned agent's CLI runs under the id the executor already wrote into the task file.
</objective>

<context>
This repository has no root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md`, the Definition of Done you are graded against (it forbids adding `replace` or `exclude` directives to `go.mod` and requires a `## Unreleased` CHANGELOG entry).

Read `docs/creating-claude-agents.md` § "5. Thread domain env through main.go" — the project's documented procedure for adding a config field and threading it to the runner. Its code example predates the `CreateTaskRunner` → `CreateAgent`/`CreateAgentProvider` split and names a function this repo no longer has; take the field-tagging convention from it, not the constructor call.

Read `pkg/factory/factory.go` — `CreateClaudeRunner` (line ~36) and `CreateClaudeSessionFactory` (line ~68) both build a `claudelib.ClaudeRunnerConfig` from the same five parameters; they must stay in step. A field added to one and not the other makes a service-mode agent silently unforced while a task-mode agent works. The two constructors that forward to them are `CreateAgent` (line ~129 → `CreateClaudeRunner` at line ~138) and `CreateAgentProvider` (line ~172 → `CreateClaudeRunner` at line ~180).

Read `main.go` — the `application` struct (line 57) carries `arg:`/`env:` tagged fields (`ClaudeConfigDir` line 62, `AgentDir` line 65, `AnthropicModel` line 89); add the new field beside `ClaudeConfigDir`. The provider call is at line 261 and the session-factory call at line 436. `main.go` never calls `CreateClaudeRunner` directly.

Read `cmd/run-task/main.go` (line ~119) — the local CLI mode calls `factory.CreateAgent` and has its own config struct, so it must pass the new parameter explicitly (an empty value).

Read `pkg/factory/factory_test.go` — it has `CreateAgentProvider` (line 19, call at 27), `CreateClaudeSessionFactory` (line 135, call at 137) and `CreateAgent` (line 142, call at 144) specs but no `CreateClaudeRunner` spec; all three call their constructors positionally, so a new parameter breaks them.

Read `main_internal_test.go` — three `createAgentProvider = func(...)` stubs (lines ~705, ~745, ~778) must match the new arity.

`github.com/bborbe/agent` is pinned at v0.139.5, whose `claude.ClaudeRunnerConfig` has no `SessionID` field and whose argv builders emit no `--session-id`. Step 0 of the requirements is the bump; do not edit this repo until `go doc github.com/bborbe/agent/claude ClaudeRunnerConfig` shows the field.

Read `README.md` § `## Env Vars` (lines ~89-112) — the table that must gain a row.
</context>

<requirements>
1. Step 0 — bump the dependency before any edit: `go get github.com/bborbe/agent@<concrete-version> && go mod tidy`, where `<concrete-version>` is the published release that adds `claude.ClaudeRunnerConfig.SessionID` and passes it to the CLI as `--session-id`. This prompt is approved only once that release exists: as of writing, the newest published version is v0.139.7 and it has neither the field nor the flag. Verify with `go doc github.com/bborbe/agent/claude ClaudeRunnerConfig` that the field exists and note its exact type; if it is still absent, stop and report a blocker — the library change belongs in `bborbe/agent`, not in this repo.
2. Add a config field to the `application` struct with `required:"false" arg:"session-id" env:"CLAUDE_SESSION_ID"` and a usage string naming the executor as the writer. Empty is valid and means "the CLI picks its own id". Every `application` field exposes both forms; the flag is the local-debug equivalent.
3. Thread the value through the whole chain: `CreateAgent` (line ~129) and `CreateAgentProvider` (line ~172) take the new parameter and forward it to `CreateClaudeRunner`; pass `a.SessionID` at `main.go:261` and at the session-factory call on `main.go:436`. Update the three `createAgentProvider = func(...)` stubs in `main_internal_test.go` (lines ~705, ~745, ~778) for the new arity, and pass `""` from `cmd/run-task/main.go:119`.
4. Set the field on the `claudelib.ClaudeRunnerConfig` in both `CreateClaudeRunner` and `CreateClaudeSessionFactory`. Keep the five shared parameters identical in shape and order; `CreateClaudeSessionFactory` appends `permissions` and keeps that last.
5. Validation is decided by what the bumped release actually does. Check the release's runner: if it rejects an invalid id before spawning and returns an error, do not duplicate the rule here — instead add the test in requirement 8 that proves the rejection. If it does not, validate here before the value reaches the config: accept only `^[A-Za-z0-9_][A-Za-z0-9_-]{0,63}$` — the library's intake pattern (`github.com/bborbe/agent/interactive/session-id.go`), which is what an executor-minted id already satisfies. Do not use the `{1,128}` file-name rule from `claude/transcript.go`: it rejects an id containing `_` that intake accepted and admits a leading `--`. Return a new `errors.Errorf(ctx, "invalid session id")` error from that call site (there is no underlying error to wrap). Name the site explicitly: validate in `application.Run` before the `createAgentProvider` call at `main.go:261`, not inside a factory — the factories are pure composition and carry no conditionals. A value starting with `--` must never reach the argv.
6. Do not read the environment anywhere else, and do not fall back to a generated id: an agent that invents its own id defeats the anchor, because the executor's recorded value is what the consumer reads.
7. Session precedence — conditional on the bumped release. Read `claude/claude-session.go` in the bumped module: v0.139.7's `func (f *sessionFactory) Create(_ string)` discards its argument, and its doc comment states the argv is identical for every id. If `Create` still discards the id, do NOT claim precedence in the doc comment and do NOT add a two-id test — state instead that the config value is the only session id and every conversation in the service shares it, and report that as a blocker, because two concurrent conversations would share one CLI identity. Only when `Create` passes the id through: state the precedence in the doc comment and add the test asserting two different `Create(id)` calls yield two different session ids.
8. Contract tests, not shape tests:
   - Drive a non-empty id through `CreateClaudeRunner` to the real subprocess boundary: put a stub `claude` script on `PATH` that writes `"$@"` to a temp file, then prints `{"type":"result","result":"ok"}` and exits 0 (the runner needs a stream-json result event to finish a turn; a bare `echo "$@"` fails the run before you can assert). Write the shim 0755 with `//nolint:gosec` — gosec is enabled. Run one turn and assert the argv contains `--session-id` followed by the id; assert an empty id produces no `--session-id` element.
   - Add a config-parse test in `main_internal_test.go` (package `main`; the external `factory_test` package cannot see `application`): set `CLAUDE_SESSION_ID`, call `argument.Parse(ctx, &application{})`, and assert the field carries it, and that unset leaves it empty. Reset `flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)` in `BeforeEach` — `argument.Parse` registers flags globally and a second Parse panics with "flag redefined" (see the existing `application argument parsing` Describe). This is what catches a typo'd `env:` tag.
   - A struct-field assertion alone does not satisfy this requirement. Update the three positional calls at `pkg/factory/factory_test.go:27` (`CreateAgentProvider`), `:137` (`CreateClaudeSessionFactory`) and `:144` (`CreateAgent`) for the new arity.
9. Add `CLAUDE_SESSION_ID` to the `## Env Vars` table in `README.md`, directly after the `CLAUDE_CONFIG_DIR` row (line ~105): `| \`CLAUDE_SESSION_ID\` | no | — | Session id the executor injects; the Claude CLI adopts it. Unset means the CLI picks its own |`
10. Add a new `## Unreleased` section directly under the intro block of `CHANGELOG.md` (above `## v0.26.9`) with one terse bullet, matching the existing `fix:`/`feat:` phrasing.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT run `go mod vendor`; bump the dependency with `go get …@vX.Y.Z && go mod tidy`.
- Do NOT add a `replace` or `exclude` directive to `go.mod`.
- Errors via `github.com/bborbe/errors` (`errors.Wrapf(ctx, err, ...)`), never `fmt.Errorf`.
- Tests are Ginkgo v2 / Gomega; extend `pkg/factory/factory_test.go` rather than adding a file.
- Factory functions stay pure composition: no conditionals, no I/O, no `context.Background()`.
- Scope is this repo only. If the library field is missing, stop and report a blocker; do not edit `bborbe/agent` from here.
- There is no spec in this repo for this change; the behavioural contract is the executor's injection plus the library's field.
</constraints>

<verification>
- `make test` passes from the repository root.
- `make precommit` passes.
- `grep -n 'CLAUDE_SESSION_ID' main.go README.md` shows the env tag and the README row.
- `grep -n 'SessionID' pkg/factory/factory.go` shows both `ClaudeRunnerConfig` constructions setting it.
- `grep -n 'session-id' pkg/factory/factory_test.go` shows the argv contract test from requirement 8 — the empty-id case is asserted there, not by a manual invocation.
</verification>
