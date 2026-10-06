---
status: completed
summary: 'Changed AnthropicAuthToken''s display tag from password to length in both main.go and cmd/run-task/main.go so the startup config log redacts the token as a character count, added a log-capturing regression spec that fails when the tag is reverted, and recorded the fix under a new ## Unreleased heading.'
execution_id: agent-claude-redact-token-exec-009-redact-anthropic-auth-token-from-startup-log
dark-factory-version: v0.196.0
created: "2026-10-06T11:39:10Z"
queued: "2026-10-06T11:39:10Z"
started: "2026-10-06T11:40:00Z"
completed: "2026-10-06T11:48:15Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. WHY A PROMPT AND NOT A DIRECT EDIT. `.dark-factory.yaml` declares this repository a
   dark-factory project, and its (untracked) `CLAUDE.md` routes every code change through
   the pipeline. The change is two struct-tag values, but it lands in a shipped binary,
   so it is a code change.

2. THE DEFECT WAS ALREADY FOUND AND DEFERRED. `prompts/completed/002-bump-agent-interactive-auth-token.md`
   REVIEWER NOTE 3 (2026-10-03) names this exact field: "OUT OF SCOPE, LIVE BUG — file
   separately … It deserves its own change." This prompt is that change. The root cause is
   the one note 2 of that same file settled for `InteractiveAuthToken`.

3. NOT A MISSING TAG — AN UNIMPLEMENTED ONE. `AnthropicAuthToken` already carries
   `display:"password"`. `Print` in github.com/bborbe/argument/v2 (argument_print.go)
   honours only `hidden` and `length`; every other value falls through to the default
   branch that logs the value verbatim. The fix changes the tag's VALUE, it does not add a tag.

4. VERIFIED LIVE 2026-10-06. `kubectlnukedev -n dev logs claude-interactive-0` (image
   agent-claude:v0.9.0) prints `Argument: AnthropicAuthToken '<value>'` in plaintext, while
   SentryDSN (92), SentryProxy and InteractiveAuthToken (64) print `length N` — they already
   carry `display:"length"`.

5. SCOPE. Rotating the exposed credential and redeploying the fixed image are operator
   actions, tracked in the vault task that spawned this prompt. Neither is part of this
   prompt, and neither is verifiable inside the container.
-->

# Redact the Anthropic auth token from the startup config log

<summary>
- The agent's startup configuration log stops printing the Anthropic auth token's value.
- The token is still reported at startup, but as a character count instead of its contents.
- Both binaries in this repository — the Kafka job entry point and the local CLI — behave the same way.
- Every other credential-bearing startup field keeps the redaction it already has.
- Non-secret settings keep printing their values exactly as today, so the startup log stays diagnostically useful.
- A test guards the redaction, so a later edit that reintroduces the leak fails the build.
- The change is recorded in the changelog.
</summary>

<objective>
Stop this binary writing the value of `ANTHROPIC_AUTH_TOKEN` into its startup configuration log, so a live credential is no longer readable by anyone with pod-log access. The token must stay observable as a length, and every other field's current rendering must be unchanged.
</objective>

<context>
This repository has no tracked root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md` (the Definition of Done you are graded against).

The startup log is produced by a shared library, not by this repository. The call chain is:

```go
// main.go
func main() {
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}
```

`service.Main` (github.com/bborbe/service, `service_main.go`) calls `argument.ParseAndPrint(ctx, app)`, which walks the exported fields of `app` and logs each one. The printer is github.com/bborbe/argument/v2 (`argument_print.go`). It honours exactly two `display` tag values:

- `display:"hidden"` — the field is skipped entirely.
- `display:"length"` — only the value's character count is logged.

Any other value falls through to the default branch, which logs the value verbatim.

Read the sibling fields in the same struct for the established pattern: `SentryDSN`, `SentryProxy` and `InteractiveAuthToken` all carry `display:"length"`.

Files you will read and change (repo-relative):
- `main.go` — the `application` argument struct (the Kafka job entry point).
- `cmd/run-task/main.go` — its own `application` struct (the local file-based CLI mode).
- `main_internal_test.go` — the argument-parsing specs; `package main`, run by the `RunSpecs` in `main_test.go`. It already imports `github.com/bborbe/argument/v2` as `argument`.
- `CHANGELOG.md` — add an `## Unreleased` section above the newest released section.

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-build-args-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-k8s-binary-conventions.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **In `main.go`, change the `AnthropicAuthToken` field's `display` tag from `password` to `length`.** The field is in the `application` struct, in the Anthropic provider-routing group next to `AnthropicBaseURL` and `AnthropicModel`. Change only the `display` value; leave `required`, `arg`, `env` and `usage` exactly as they are.

2. **In `cmd/run-task/main.go`, make the identical change** on that file's own `AnthropicAuthToken` field. Both structs carry the defect; fixing only one leaves the other leaking.

3. **Audit every other field in both structs** for a secret-bearing value whose `display` tag is absent or set to a value the printer does not honour, and classify each in your final report. Assess these explicitly rather than by assumption:
   - `ClaudeEnvRaw`, `EnvContextRaw` — comma-separated `KEY=VALUE` bags. They can carry an arbitrary credential (the existing spec in `main_internal_test.go` passes `GH_TOKEN=abc` through `ClaudeEnvRaw`), so decide deliberately whether each is secret-bearing.
   - `SentryDSN`, `SentryProxy`, `InteractiveAuthToken` — already `display:"length"`; do not change them.
   - Every remaining field (`AnthropicBaseURL`, `AnthropicModel`, `AllowedToolsRaw`, `TaskContent`, `KafkaBrokers`, `TaskID`, `Branch`, `TopicPrefix`, `Phase`, `AgentType`, `Listen`, `ProviderBaseURL`, `PushgatewayURL`, `TaskType`, `ClaudeConfigDir`, `AgentDir`, `A2AAgentName`, `A2APublicURL`, and any others present) — expected non-secret; confirm rather than assume.

   Report each classification in your final report. Change no field's tag other than `AnthropicAuthToken` — the summary promises every other credential-bearing field keeps the redaction it already has, so any further redaction is a separate change, not this one. If your audit finds a genuinely secret-bearing field with no working redaction, say so explicitly in the report as a follow-up rather than fixing it here.

4. **Add a regression test that fails if the token's value reaches the log.** Put it in `main_internal_test.go`, in the existing `package main` suite. Redirect the standard library logger's output to a buffer for the duration of the spec, call `argument.Print` on an `application` whose `AnthropicAuthToken` holds a distinctive sentinel value, then assert that the sentinel does **not** appear in the captured output while the field name `AnthropicAuthToken` **does**. Restore the logger's output when the spec ends. This is the boundary the change crosses: the printer is a library, so a struct-tag assertion alone would prove only that a string was typed, not that the library redacts it. `cmd/run-task` declares its own `application` type in its own `main` package, so add the equivalent spec in that package's `_internal_test.go` if one exists there; if it does not, state in your final report that `cmd/run-task` is guarded only by verification 5.

5. **Add an `## Unreleased` section to `CHANGELOG.md`** above the newest released section, matching the existing bullet shape:

   ```
   ## Unreleased

   - fix: the Anthropic auth token is no longer printed in plaintext by the startup configuration log — `display:"password"` is not a value the argument printer honours, so the token fell through to the default branch and was logged verbatim; it now uses `display:"length"`, matching `INTERACTIVE_AUTH_TOKEN` and the Sentry fields
   ```

   Do not touch any released section.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) both `main.go` and `cmd/run-task/main.go` carry `display:"length"` on their `AnthropicAuthToken` field and neither still contains `display:"password"`; (ii) no non-secret field's rendering changed; and (iii) the new spec genuinely fails when the tag is reverted to `password` — prove this by temporarily reverting the tag, running the spec, and restoring the fix.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do NOT change `github.com/bborbe/argument` — the library behaves as documented; the call site is what is wrong. Do not add a `replace` directive and do not vendor a fork.
- Do NOT change `SentryDSN`, `SentryProxy` or `InteractiveAuthToken` — they already carry `display:"length"`.
- Do NOT change the `arg`, `env`, `required` or `usage` tags of `AnthropicAuthToken`; only its `display` value changes. The env var name `ANTHROPIC_AUTH_TOKEN` is fixed — it is what the deployment supplies.
- Do NOT use `display:"hidden"` on `AnthropicAuthToken`. The field must stay observable as a length; hiding it would remove the startup log's ability to show whether the deployment supplied a value at all.
- Do NOT redact non-secret fields. The startup log is the primary diagnostic for a misconfigured pod; blanket redaction costs more than it protects.
- Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command. `vendor/` is a build-time artifact, gitignored, and wiped by `make precommit`.
- Errors use `github.com/bborbe/errors` — never `fmt.Errorf`, never a bare `return err`.
- Tests use Ginkgo v2 / Gomega. Do not create new test files.
- Run `make precommit` at the repository root; there is no per-package Makefile.
</constraints>

<verification>
Run each of these and confirm the stated result.

1. `ROOTDIR=/workspace make precommit` — exits 0.
2. `! grep -q 'display:"password"' main.go` — exits 0 (no field in the job entry point still carries the unimplemented tag).
3. `! grep -q 'display:"password"' cmd/run-task/main.go` — exits 0.
4. `grep -cE 'AnthropicAuthToken string.*display:"length"' main.go` — prints `1`.
5. `grep -cE 'AnthropicAuthToken string.*display:"length"' cmd/run-task/main.go` — prints `1`.
6. `! grep -rq 'display:"password"' --include='*.go' .` — exits 0 (no Go file in the repository still carries the unimplemented tag; this covers any field outside the two files above).
7. `grep -n '^## Unreleased' CHANGELOG.md` — prints exactly one line.
</verification>
