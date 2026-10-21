---
status: completed
summary: 'Guarded application.Run against a nil agent.Run result by extracting recordAgentResult, added a package-level createAgentProvider seam with three regression specs, and recorded the fix under ## Unreleased.'
execution_id: agent-claude-nil-result-guard-exec-026-nil-result-guard
dark-factory-version: v0.198.7
created: "2026-10-21T12:53:18Z"
queued: "2026-10-21T12:53:18Z"
started: "2026-10-21T12:55:26Z"
completed: "2026-10-21T13:08:28Z"
---

# Nil agent result fails the run instead of panicking

<summary>
- A task-routed run that produces no result now ends with a named error instead of a nil-pointer panic.
- A run whose phase has nothing left to do comes back with no result, and the process currently dies with a panic instead of reporting a failed run.
- This is a real, reachable outcome, not a hypothetical: it happens whenever a task's body already carries the phase's output section, so every step is skipped.
- The panic depends only on task state, so it recurs on every re-drive: it burns a Job attempt and can exhaust the retry budget, and the run is lost with no result and no error to report.
- The guard reports the condition as a failed run, so the existing metrics and delivery paths see a failure rather than a crash.
- The regression test exercises the real production call path rather than a helper beside it, so it proves the fix where the crash occurred.
- The new test fails with the panic before the fix and passes after it.
- No behaviour change on any path that already returns a result.
- A changelog entry records the fix.
</summary>

<objective>
Make `application.Run` report a failed run with a named error when `agent.Run` returns a nil result and a nil error, instead of dereferencing it and panicking, so a `(nil, nil)` costs one recorded failure rather than a lost Job attempt.
</objective>

<context>
Read `docs/dod.md` first — it is this repository's declared `validationPrompt` and the Definition of Done you are graded against. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `autoRelease: false`, `validationPrompt: docs/dod.md`; the effective config also resolves `validationCommand: make precommit`.

This is a standalone prompt: there is no spec file to read.

**The defect, precisely.** In `main.go`, `application.Run` (the method starting at the line `func (a *application) Run(ctx context.Context, _ libsentry.Client) error`) does:

```go
result, err := agent.Run(ctx, a.Phase, a.TaskContent, deliverer)
if err != nil {
    ...
    return errors.Wrap(ctx, err, "agent run failed")
}
jobMetrics.RecordRun(result.Status)
```

`err == nil` does not imply `result != nil`. In `github.com/bborbe/agent` (pinned at `v0.103.1` in `go.mod`), `Agent.Run` declares `var lastResult *Result` and returns `lastResult, nil` when the first phase's steps all report `ShouldRun == false` — that is exit condition 1, "result == nil — empty Steps or all ShouldRun=false". So `(nil, nil)` reaches `main.go` and the `result.Status` dereference panics.

Read these files before editing:

- `main.go` — `application.Run`, the provider construction inside it (`provider := factory.CreateAgentProvider(...)`), the `agent.Run` call, and the `var inClusterNamespacePath = ...` package-level variable further down the file. That variable is the in-tree exemplar for the seam this prompt adds.
- `pkg/factory/factory.go` — `CreateAgentProvider`, for its exact signature and return type (`agentlib.AgentProvider`).
- `main_internal_test.go` — the existing Ginkgo suite in `package main`. It already imports `claudelib "github.com/bborbe/agent/claude"` and `agentmocks "github.com/bborbe/agent/mocks"`; follow its `Describe` / `BeforeEach` shape. It does **not** import the root `agentlib "github.com/bborbe/agent"` package today — requirement 4 adds it.
- `CLAUDE.md` — the repo's own rules: errors via `github.com/bborbe/errors`, Ginkgo v2 / Gomega with counterfeiter mocks, Interface → Constructor → Struct → Method, and "do NOT commit". The constraints below restate what it already requires.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement, the required conventional prefix, and the frozen-preamble rule that requirement 5 follows.
- `CHANGELOG.md` — the first `## ` heading is `## v0.22.0`; there is no `## Unreleased` section today.

### Why the seam is required

`application.Run` builds its agent through `factory.CreateAgentProvider(...)` and stores the result in a local variable. A test in `package main` cannot substitute that call, so without a seam the only testable surface would be a helper extracted beside the call site — and a test against a helper would pass while the real `main.go` dereference stayed unguarded. The seam is what makes the test prove the production path.

### What is out of scope

No change to `github.com/bborbe/agent` or any other module, and no `go.mod` / `go.sum` change. No change to `pkg/factory`, `pkg/envbag`, `pkg/heartbeat`, `pkg/permission`, `pkg/prompts`. No change to `agent.Run`'s call signature, the delivery path, the metrics names, or `application.runService`. The sibling entry point `cmd/run-task/main.go`'s `application.Run` is out of scope: it never dereferences `result`, and `agentlib.PrintResult` is nil-safe, so a `(nil, nil)` there returns success rather than panicking — that silent-success behaviour is a separate defect, not this panic. No new file, and no change to any existing file other than `main.go`, `main_internal_test.go` and `CHANGELOG.md`.
</context>

<requirements>
1. **Add the package-level seam in `main.go`.** Declare, beside the existing `var inClusterNamespacePath = ...` package-level variable:

   ```go
   // createAgentProvider is the seam tests use to inject an agent provider.
   // Production uses factory.CreateAgentProvider.
   var createAgentProvider = factory.CreateAgentProvider
   ```

   Give it a doc comment in the file's existing style. The name must be `createAgentProvider` — it mirrors `factory.CreateAgentProvider`, which is what the declaration aliases.

   **This is a deliberate, recorded deviation.** `go-composition.md` (at `/home/node/.claude/plugins/marketplaces/coding/docs/go-composition.md`) warns against test-only package-level mutable state (`var X = default` overridden by tests) and prefers a constructor field. The package variable is the required shape here: `main.go` already carries the same pattern at `var inClusterNamespacePath = ...`, and verification items 1–3 and 7 and the success criteria all assert this variable. `argument.Parse` does skip untagged struct fields, so a constructor field would also compile — do not substitute one; it fails those checks. The test must reach `application.Run`, not a helper.

2. **Call the seam instead of the factory directly.** Inside `application.Run`, change the provider construction from `provider := factory.CreateAgentProvider(...)` to `provider := createAgentProvider(...)`, keeping the argument list byte-for-byte identical. Do not change the variable name `provider` or the `provider.Get(...)` call below it.

3. **Guard the nil result — by extracting the tail, not by adding a line.** `application.Run` is already at the `funlen` ceiling: its body is exactly 80 lines today and `.golangci.yml` sets `funlen.lines: 80`, so the `lint` target of `make precommit` rejects any inline addition (measured against this repo's own config: a five-line guard measures 85, or 86 with a blank separator, against `funlen.lines: 80`). Extract the post-`agent.Run` tail into a new helper method on `application`, and place the helper immediately **after** `application.Run` in `main.go`:

   ```go
   func recordAgentResult(
       ctx context.Context,
       jobMetrics libmetrics.JobMetrics,
       start time.Time,
       result *agentlib.Result,
       runErr error,
   ) error {
       if runErr != nil {
           jobMetrics.RecordRun(agentlib.AgentStatusFailed)
           jobMetrics.RecordDuration(time.Since(start))
           return errors.Wrap(ctx, runErr, "agent run failed")
       }
       if result == nil {
           jobMetrics.RecordRun(agentlib.AgentStatusFailed)
           jobMetrics.RecordDuration(time.Since(start))
           return errors.New(ctx, "agent run returned no result")
       }
       jobMetrics.RecordRun(result.Status)
       jobMetrics.RecordDuration(time.Since(start))
       return agentlib.PrintResult(ctx, result)
   }
   ```

   Then replace the tail of `application.Run` — from the `if err != nil {` line through `return agentlib.PrintResult(ctx, result)` — with the single line `return recordAgentResult(ctx, jobMetrics, start, result, err)`. Keep the `result, err := agent.Run(ctx, a.Phase, a.TaskContent, deliverer)` line in `Run` unchanged; only the handling moves.

   Inside the helper the nil guard sits between the `runErr != nil` block and the `jobMetrics.RecordRun(result.Status)` line, and the metrics pair matches every other failure path. The error must come from `github.com/bborbe/errors` as `errors.New(ctx, "...")` — never `fmt.Errorf`, never a bare `return err`, never a nil error. The message must contain the exact substring `agent run returned no result`.

4. **Add the regression test to `main_internal_test.go`.** Add one `Describe("application.Run nil result", ...)` block in the file's existing Ginkgo style that:

   a. Saves and restores `createAgentProvider` — assign the stub in `BeforeEach` and restore the original with `DeferCleanup`, which is the file's existing pattern for package-level seams (see `main_internal_test.go` around lines 635–637), so no other spec sees the stub.
   b. Sets `createAgentProvider` to a function returning a provider whose `Get` yields an agent that genuinely returns `(nil, nil)`. Build that agent from the library's own exported types so the input is the real shape, not a manufactured one: `agentlib.NewAgent(agentlib.NewPhase(<the phase under test>, <a Step whose ShouldRun returns false>))`. Prefer the library's counterfeiter fakes — `agentmocks.AgentStep` (`ShouldRunReturns(false, nil)`) for the step and `agentmocks.AgentAgentProvider` (`GetReturns(...)`) for the provider — since `main_internal_test.go` already imports `agentmocks` and the project mandates counterfeiter mocks over hand-written ones. Read `github.com/bborbe/agent`'s `agent_step.go` and `agent_phase.go` before writing it. The stub must have exactly `factory.CreateAgentProvider`'s signature — `func(claudelib.ClaudeConfigDir, claudelib.AgentDir, claudelib.AllowedTools, claudelib.ClaudeModel, map[string]string, map[string]string) agentlib.AgentProvider` — so write it as a func literal with `_` parameter names. If the returned agent cannot be made to return `(nil, nil)` this way, stop and report that rather than substituting a fake that bypasses the library.
   c. Constructs an `application` whose fields let `Run` reach the agent call without external services: `TaskID` empty (so `Run` uses `delivery.NewNoopResultDeliverer()` and never opens Kafka), `AgentType` not `factory.AgentTypeService`, `TaskContent` non-empty, `Phase` set to the phase the injected agent serves, and `TaskType` set to the key the injected provider serves (or a provider fake that ignores the task type). `Run` passes `agentlib.TaskType(a.TaskType)` to `provider.Get`, and `agentlib.NewAgentProvider` errors on a map miss — with an empty `TaskType` the spec would fail with `select agent for task_type` before `agent.Run` is ever reached.
   d. Calls `a.Run(ctx, nil)` and asserts it returns a non-nil error containing `agent run returned no result`, and that it did **not** panic. Wrap the call so a panic fails the spec with a clear message rather than crashing the suite — `Expect(func() { _ = a.Run(ctx, nil) }).NotTo(Panic())` followed by the separate call that captures the error, or an equivalent explicit form.
   e. Asserts the returned error is non-nil — this is the discriminating half: before the guard the same call panics, after it the call returns an error.
   f. Adds a second spec in the same `Describe` covering the `runErr != nil` branch of `recordAgentResult`: an `agentmocks.AgentStep` with `NameReturns("boom")`, `ShouldRunReturns(true, nil)` and `RunReturns(nil, errors.New(ctx, "boom"))`, driven through the same `a.Run` call, asserting the returned error contains `agent run failed` and that the seam is restored. This branch is moved, previously untested code, so the new helper would otherwise sit well under the DoD's 80% coverage target.

5. **Add a `## Unreleased` changelog section.** `CHANGELOG.md` has no `## Unreleased` heading today; the first `## ` heading is `## v0.22.0`. Insert `## Unreleased` immediately above `## v0.22.0`, holding exactly one `fix:` bullet. It must use the literal word `dereferenced` (verification item 14 greps for it) and state that `application.Run` dereferenced the result of `agent.Run` without checking it, so a `(nil, nil)` return — which `github.com/bborbe/agent` produces when every step of a phase is skipped — panicked the binary and burned a Job attempt; that the run now records a failed run and returns a named error instead; and that the agent provider is reachable through a package-level seam so a test can drive the production call site. Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes. Do not move, delete or reorder any existing section or bullet.

6. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) `main.go` still compiles and `make precommit` exits 0; (ii) the guard sits between the `err != nil` block and the `RecordRun(result.Status)` line; (iii) the new spec fails with a panic when the guard is removed and passes with it in place — verify this by temporarily reverting only the guard locally, running `ROOTDIR=/workspace go test -mod=mod -run TestSuite . -count=1 -args -ginkgo.focus='application.Run nil result'`, and restoring it; (iv) no other spec changed behaviour.
</requirements>

<constraints>
- **Do NOT commit — dark-factory handles git.** Do not run any `git` command; the container may be started with `.git` masked, so every `git` invocation fails, and a failed command in `<verification>` is a false-positive pass because the executor does not check verification exit codes.
- **The guard is the change.** Do not "fix" this by changing `github.com/bborbe/agent`, by vendoring it, by adding a `replace` directive, or by editing `go.mod` / `go.sum`. The library defect is tracked separately.
- **The test must reach the production call site.** A test against an extracted helper does not satisfy requirement 4 and will be rejected — the whole point of the seam in requirement 1 is that `application.Run` itself is the thing under test.
- **Restore the seam.** `createAgentProvider` is a package-level variable shared by every spec in the binary. A spec that leaves it stubbed changes what later specs exercise.
- **Error style.** `github.com/bborbe/errors` only — `errors.New(ctx, "msg")`, `errors.Wrap(ctx, err, "msg")`. Never `fmt.Errorf`, never a bare `return err`.
- **Existing tests must still pass.** `ROOTDIR=/workspace make precommit` must exit 0. `make precommit` runs `format`, `generate` and `addlicense`, which can rewrite `.go` files with no source change behind them — leave such a rewrite alone; do not revert it by hand and do not treat it as this prompt's work.
- **No new package, binary, symlink, alias, or credential.** Any test-local helper (including a `Step` implementation, if one is needed) lives in `main_internal_test.go`.
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does.

A note on reading the results: `grep -c` is used only where the expected count is non-zero; absence checks are written as `! grep -q`, which prints nothing and exits 0.

1. `grep -c 'var createAgentProvider = factory.CreateAgentProvider' main.go` — prints `1` (the seam exists).
2. `grep -c 'provider := createAgentProvider(' main.go` — prints `1` (the call site uses the seam).
3. `! grep -q 'provider := factory.CreateAgentProvider(' main.go` — exits 0 (the direct factory call is gone from the call site).
4. `grep -c 'agent run returned no result' main.go` — prints `1` (the named error exists).
5. `awk '/result, err := agent.Run/{r=NR} /if result == nil/{n=NR} /jobMetrics.RecordRun\(result.Status\)/{s=NR} END{print (n>r && n<s) ? "guard-position-ok" : "guard-position-WRONG"}' main.go` — prints `guard-position-ok`. **This is the discriminating structural check**: it fails while the dereference is still unguarded.
6. `grep -c 'application.Run nil result' main_internal_test.go` — prints `1` (the regression spec exists).
7. `grep -c 'createAgentProvider = ' main_internal_test.go` — prints at least `2` (the spec stubs it and restores it).
8. `! grep -q 'fmt.Errorf' main.go main_internal_test.go` — exits 0 (error style held).
9. `ROOTDIR=/workspace go test -mod=mod -v -run TestSuite ./... -count=1 -args -ginkgo.v` — exits 0 and prints `application.Run nil result` in the Ginkgo spec listing. `-v` and `-ginkgo.v` are required because `go test` suppresses a passing package's stdout, so without them the spec listing is not observable and a selection error would still exit 0. `TestSuite` is the bootstrap function in `main_test.go`; `-mod=mod` is required because this repository does not commit `vendor/`.
10. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–5.
11. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
12. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
13. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (exactly one bullet under the new section).
14. `awk '/^## /{sec=$0} /agent run returned no result|dereferenced/{print "sits under: " sec}' CHANGELOG.md | head -1` — prints `sits under: ## Unreleased` (the bullet did not fold into a released section).
</verification>

<success_criteria>
- `main.go` declares `var createAgentProvider = factory.CreateAgentProvider` as a package-level seam with a doc comment.
- `application.Run` builds its provider through that seam and no longer calls `factory.CreateAgentProvider` directly.
- `application.Run` returns a `github.com/bborbe/errors` error containing `agent run returned no result` when `agent.Run` returns a nil result and a nil error, after recording the failed-run metrics and before any `result.Status` dereference.
- `main_internal_test.go` carries a spec that stubs the seam, drives `application.Run` with an agent that genuinely returns `(nil, nil)`, and asserts a non-nil error with that message and no panic — and restores the seam afterwards.
- The spec fails with a panic when the guard is removed and passes with it in place.
- `make precommit` exits 0.
- `CHANGELOG.md` gains a `## Unreleased` section with exactly one `fix:` bullet, placed above `## v0.22.0`, and no existing section or bullet changes.
- No `go.mod`, `go.sum`, or `vendor/` change, and no `replace` or `exclude` directive.
</success_criteria>
