---
status: completed
spec: [bug-nuke-prod-cf]
summary: Added a fail-fast validateTaskType guard in main.go for empty/unset TASK_TYPE, with parser and helper specs, a README env-var row, and an Unreleased changelog entry; make precommit exits 0.
execution_id: 5fd3ba5d-a9d2-5e5c-9889-53cb34ef0451-exec-026-bug-nuke-prod-cf-fail-fast-empty-task-type
dark-factory-version: dev
created: "2026-10-23T05:31:50Z"
queued: "2026-10-23T06:04:12Z"
started: "2026-10-23T06:04:15Z"
completed: "2026-10-23T06:21:37Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. WHY THIS PROMPT EXISTS. Sentry issue NUKE-PROD-CF: a task publisher dispatched
   a job to agent-claude with an empty task_type, and the dispatch table (which
   registers only llm, healthcheck and oauth-probe) produced a generic map-miss
   error. The spec's Recommended Fix points at the executor/task-controller — a
   DIFFERENT repository that is not present in this checkout. A prompt executed
   by dark-factory in THIS repository cannot change it. That cross-repo half is
   therefore surfaced here as an open question, never as a deliverable.

2. THE DECISION. The bug is not fixable at its root here, but the symptom can be
   made fail-fast and diagnosable here, which is the smallest defensible in-repo
   change. The library's error contract for a genuinely unknown non-empty value
   (`unknown task_type %q for %s; accepted: %v`) is preserved untouched; only the
   two never-legitimate values — the empty string and the flag's "unknown"
   default — get a new, actionable message that names TASK_TYPE, the publisher
   that must stamp it, and the agent.TaskType* constants.

3. WHY BOTH "" AND "unknown". Verified in the argument library: an env var that
   is PRESENT but empty overrides the default and yields "" (`argument_env.go`,
   `envToValues`, then `argument_parse.go`, `mergeValues(default, env, args)`);
   an ABSENT env var falls back to the `default:"unknown"` tag. So the publisher
   bug can reach `provider.Get` as either value, and both are map misses today.
   Rejecting the "unknown" sentinel is safe: it is the default tag, not a value
   any publisher may legitimately send (the accepted set is [healthcheck llm
   oauth-probe]), and every job carrying it already failed before this change.

4. WHAT IS DELIBERATELY NOT DONE. No fallback that maps empty/unset task_type to
   the LLM agent (that would run the wrong workload). No change to
   `default:"unknown"` (it is a Prometheus grouping label; changing it would
   alter every existing `task_type` series and is not needed to fail fast). No
   edit to `pkg/factory` (its error contract is pinned by
   `pkg/factory/factory_test.go`). No edit to the external agent library (no
   replace, fork or vendor — `docs/dod.md` forbids replace directives).

5. OPEN QUESTIONS THE REVIEWER OWNS.
   a. The root fix belongs to task/executor: every dispatched task must carry a
      non-empty task_type from the agent.TaskType* constants. That audit is not
      deliverable in this repository; someone must open it there.
   b. Whether rejecting the "unknown" sentinel is wanted, or only the empty
      string. This prompt rejects both because both are guaranteed failures.
   c. The spec says "agent-claude itself requires no code change". This prompt
      intentionally deviates: it hardens the failure surface in-repo. If the
      reviewer prefers a pure audit with no code change, this prompt should be
      rejected rather than half-applied.
   d. Whether the publisher sets TASK_TYPE to "" or omits it is unverified; the
      guard covers both, so the answer does not change the implementation.
-->

# Fail fast when a task arrives without a task_type

<summary>
- A job that reaches this binary without a real task type now fails immediately with a message that says what is missing and who must supply it.
- The message names `TASK_TYPE`, the task publisher, and the `agent.TaskType*` constants, instead of a generic "unknown task_type" map miss.
- Both ways the value can arrive are covered: `TASK_TYPE` set to the empty string, and `TASK_TYPE` absent so the flag's `unknown` default applies.
- A genuinely unknown but non-empty value (for example `bogus`) keeps exactly the error it has today, including the quoted value and the sorted accepted-types list.
- Nothing is silently defaulted: no empty task type is ever routed to the LLM agent, so the wrong workload can never run.
- The dispatch table itself is unchanged, and its existing tests keep passing untouched.
- The accepted task types are now documented in the README environment table, where `TASK_TYPE` was previously absent.
- A changelog entry records the hardening, so the release that follows carries it.
- The root cause remains in the task publisher (a different repository); this change makes its symptom diagnosable from the pod's error alone.
</summary>

<objective>
Reject a task-routed job whose `task_type` was never stamped — the empty string or the `TaskType` flag's `unknown` default — with an actionable error naming `TASK_TYPE` and the publisher that must set it, while leaving the existing `unknown task_type` error contract for genuinely unknown non-empty values exactly as it is.
</objective>

<context>
Read `docs/dod.md` first — it is the Definition of Done you are graded against and this repository's declared `validationPrompt`. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `autoRelease: false`, `autoGeneratePrompts: true`, `defaultBranch: master`. `make precommit` is the validation command (the resolved config's default `preflightCommand`); `ROOTDIR=/workspace` pins the repository root on every `make` call.

This prompt is derived from `specs/in-progress/bug-nuke-prod-cf.md`. The spec's Recommended Fix points at the executor/task-controller, which is a different repository and not present in this checkout; that audit is **out of scope here** and is recorded as a reviewer question in the comment at the top of this file. Do not attempt it, and do not invent cross-repo edits.

Read these files before editing:

- `main.go` — the whole file. `Run` is the only place a task-routed job selects an agent.
  - `application.TaskType` is declared at line 176:
    `TaskType string \`required:"false" arg:"task-type" env:"TASK_TYPE" usage:"Task type label for metric grouping" default:"unknown"\``
  - The task-routed guard for `TASK_CONTENT` is at lines 226–234; it records `jobMetrics.RecordRun(agentlib.AgentStatusFailed)` and `jobMetrics.RecordDuration(...)` before returning. New guards follow that shape.
  - The dispatch call is at line 244: `agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))`, wrapped at line 248 as `errors.Wrap(ctx, err, "select agent for task_type")`.
  - The service-agent branch (`a.AgentType == factory.AgentTypeService`) returns at lines 223–225, **before** the task-routed guards. A service agent has no task and must not be affected.
- `pkg/factory/factory.go` — `CreateAgentProvider` (lines 165–188). It is pure composition: it builds one `ClaudeRunner`, the domain agent, the liveness agent, and returns `agentlib.NewAgentProvider(serviceName, map[agentlib.TaskType]*agentlib.Agent{agentlib.TaskTypeLLM: domainAgent, agentlib.TaskTypeHealthcheck: livenessAgent, agentlib.TaskTypeOAuthProbe: livenessAgent})`. Do **not** edit this file.
- `pkg/factory/factory_test.go` — the pinned error contract (lines 61–87) for `provider.Get(ctx, agentlib.TaskType("bogus"))`: the message must contain `unknown task_type`, the offending value quoted (`"bogus"`), the binary name (`agent-claude`), and the sorted accepted-types list (`[healthcheck llm oauth-probe]`). Those specs must keep passing unchanged.
- `main_internal_test.go` — the existing internal specs. Note the `BeforeEach` that resets the global flag set (`flag.CommandLine = flag.NewFlagSet(...)`, `os.Args = []string{"agent-claude"}`) before any `argument.Parse`, and the `DeferCleanup` pattern for env vars. New specs belong here because `application` and the new helper are unexported.
- `README.md` — the `## Env Vars` table at lines 89–111. It documents `TASK_CONTENT`, `TASK_ID`, `KAFKA_BROKERS` and others, but has **no `TASK_TYPE` row**.
- `CHANGELOG.md` — line 1 is `# Changelog`; lines 2–8 are the frozen preamble; the first `##` heading is `## v0.22.0` at line 9. There is currently **no** `## Unreleased` section.
- `docs/dod.md`, and the coding guides at `/home/node/.claude/plugins/marketplaces/coding/docs/`: `go-error-wrapping-guide.md`, `go-testing-guide.md`, `go-doc-best-practices.md`, `changelog-guide.md`.

### Verified facts about how the value arrives

The `github.com/bborbe/argument/v2` parser (v2.13.2) merges defaults, then env, then args (`argument_parse.go`, `ParseOnly` → `mergeValues(defaultValues, envValues, argsValues)`):

- `TASK_TYPE` **absent** → the `default:"unknown"` tag supplies `"unknown"` (`argument_default.go`, `DefaultValues`, string branch).
- `TASK_TYPE` **present and empty** (`TASK_TYPE=`) → `envToValues` sets `""`, which overrides the default, so the field is `""` (`argument_env.go`, `envToValues`, `if !ok { continue }` — presence, not non-emptiness, is what it checks).

Both values are map misses in the dispatch table. With the library as-is, `agentlib.NewAgentProvider(...).Get` (`agent_agent-provider.go` v0.103.1, lines 43–57) produces:

`unknown task_type "" for agent-claude; accepted: [healthcheck llm oauth-probe]`

or

`unknown task_type "unknown" for agent-claude; accepted: [healthcheck llm oauth-probe]`

Neither names the environment variable nor the component that must set it. The external library is **not** editable here (no `replace`, fork or vendor — `docs/dod.md` forbids replace directives), so the guard belongs in `main.go`.

### What is out of scope

No change to `pkg/factory/factory.go`, `pkg/factory/factory_test.go`, `cmd/run-task/main.go`, `go.mod`, `go.sum`, `Dockerfile`, `Makefile*`, `.dark-factory.yaml`, or any file under `agent/`, `scripts/`, `docs/` other than `README.md` and `CHANGELOG.md`. No fallback that maps an empty/unset task type to any agent. No change to the `default:"unknown"` tag. No audit or edit of task/executor.
</context>

<requirements>
1. **Add the guard to `main.go`.** Define one package-level constant and one package-level function, near the other helpers (for example after `buildClaudeEnv`, before `runService`):

   ```go
   // taskTypeUnset is the value the TaskType flag's default tag supplies when
   // TASK_TYPE is absent. It is a metrics-grouping placeholder, never a
   // dispatched task type: the dispatch table registers only llm, healthcheck
   // and oauth-probe. It must stay equal to the TaskType field's
   // `default:"unknown"` tag.
   const taskTypeUnset agentlib.TaskType = "unknown"

   // validateTaskType rejects a task_type the publisher never stamped. Both the
   // empty string (TASK_TYPE present but empty) and the flag's default (TASK_TYPE
   // absent) are map misses in the dispatch table, so without this guard the
   // operator sees a generic "unknown task_type" error that names neither the
   // missing environment variable nor the component that must set it. A
   // genuinely unknown non-empty value is left to the provider, whose error
   // lists the accepted types.
   func validateTaskType(ctx context.Context, taskType agentlib.TaskType) error {
   	switch taskType {
   	case "":
   		return errors.Errorf(
   			ctx,
   			"TASK_TYPE is empty; the task publisher (task/executor) must stamp one of the agent.TaskType* constants before dispatching a job to agent-claude",
   		)
   	case taskTypeUnset:
   		return errors.Errorf(
   			ctx,
   			"TASK_TYPE is unset (default %q); the task publisher (task/executor) must stamp one of the agent.TaskType* constants before dispatching a job to agent-claude",
   			string(taskTypeUnset),
   		)
   	}
   	return nil
   }
   ```

   `agentlib` and `errors` are already imported by `main.go`; do not add an import. Do not use `fmt.Errorf` and do not call `context.Background()`.

2. **Wire the guard into `Run`, after the `TASK_CONTENT` guard and before the provider is constructed.** Immediately after the block at lines 226–234 (the `TASK_CONTENT` guard) and before `provider := factory.CreateAgentProvider(...)`, add:

   ```go
   	if err := validateTaskType(ctx, agentlib.TaskType(a.TaskType)); err != nil {
   		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
   		jobMetrics.RecordDuration(time.Since(start))
   		return errors.Wrap(ctx, err, "validate task_type")
   	}
   ```

   Order is load-bearing: the check must run **after** the `AGENT_TYPE=service` branch (a service agent has no task) and **before** `factory.CreateAgentProvider`, so a mis-stamped job fails without building a provider or touching the dispatch table. The `jobMetrics` calls mirror the sibling guards.

3. **Do not change the existing dispatch behaviour.** Leave `pkg/factory/factory.go` and `pkg/factory/factory_test.go` untouched. In particular, a non-empty value that is not in the table (for example `bogus`) must still reach `provider.Get` and produce the library error containing `unknown task_type`, the quoted value, `agent-claude`, and `[healthcheck llm oauth-probe]`.

4. **Add specs to `main_internal_test.go`.** Add two `Describe` blocks, following the file's existing `BeforeEach` reset of `flag.CommandLine` and `os.Args` before any `argument.Parse`.

   a. `Describe("TASK_TYPE parsing")` — pin how the value arrives, through the real parser:
   - `It("defaults to unknown when TASK_TYPE is absent")`: `Expect(os.Unsetenv("TASK_TYPE")).To(Succeed())` with a `DeferCleanup` that restores nothing (unset is the desired end state; add one only if the file's convention requires it), parse `&application{}` with `argument.Parse(ctx, app)`, expect no error and `app.TaskType == "unknown"`.
   - `It("keeps the empty string when TASK_TYPE is set empty")`: `Expect(os.Setenv("TASK_TYPE", "")).To(Succeed())` with `DeferCleanup(func() { _ = os.Unsetenv("TASK_TYPE") })`, parse, expect no error and `app.TaskType == ""`.

   These two specs are the boundary test: they fail if the `default` tag or the parser's present-but-empty handling changes, which is exactly what would silently reopen the bug.

   b. `Describe("validateTaskType")` — a table of cases calling the helper directly:
   - `agentlib.TaskType("")` → error; `err.Error()` contains `TASK_TYPE is empty` and `task publisher`.
   - `taskTypeUnset` (or `agentlib.TaskType("unknown")`) → error; contains `TASK_TYPE is unset` and `"unknown"`.
   - `agentlib.TaskType("bogus")` → **nil** (the provider, not this helper, owns that error).
   - `agentlib.TaskTypeLLM`, `agentlib.TaskTypeHealthcheck`, `agentlib.TaskTypeOAuthProbe` → nil each.

   Use Ginkgo v2 / Gomega as the file already does. Do not add a test that calls `application.Run`: its deferred Pushgateway push has no deadline and would make the suite flaky.

5. **Document `TASK_TYPE` in `README.md`.** Add one row to the `## Env Vars` table, immediately after the `| \`KAFKA_BROKERS\` | ... |` row and before the `| \`SENTRY_DSN\` | ... |` row:

   `| \`TASK_TYPE\` | yes (unless \`AGENT_TYPE=service\`) | \`unknown\` | Task type the executor stamps for dispatch and metric grouping: one of \`llm\`, \`healthcheck\`, \`oauth-probe\`. A task-routed job whose value is empty or unset fails fast; the task publisher must stamp it |`

   Do not restructure the table and do not edit any other row.

6. **Add a `## Unreleased` changelog entry.** `CHANGELOG.md` currently has no `## Unreleased` section. Insert one **directly above** `## v0.22.0` (line 9), leaving the frozen preamble at lines 1–8 untouched, with exactly one `fix:` bullet. It must state: a task-routed job whose `TASK_TYPE` is empty or absent now fails fast with a message naming the variable, the task publisher and the `agent.TaskType*` constants, instead of the generic `unknown task_type` map miss; both arrival paths are covered (`TASK_TYPE=` present-empty overrides the default, an absent `TASK_TYPE` falls back to the `unknown` default tag); the existing error for a genuinely unknown non-empty value is unchanged; and no fallback routes a missing task type to any agent. One long sentence in the style of the existing entries: backticked identifiers, em dashes. Do not move, delete or reorder any other section.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result.
</requirements>

<constraints>
- **Do NOT commit — dark-factory handles git.** Do not run any `git` command; the container may be started with `.git` masked, so every `git` invocation fails, and a failed command in `<verification>` is a false-positive pass because the executor does not check verification exit codes.
- **No fallback.** Never map an empty or unset `task_type` to `agentlib.TaskTypeLLM` or any other agent. Running the LLM workload on a job that never asked for it is the failure this change exists to prevent.
- **Preserve the dispatch-table error contract.** `pkg/factory/factory_test.go` pins `unknown task_type`, the quoted offending value, `agent-claude`, and `[healthcheck llm oauth-probe]` for a non-empty unknown value. Do not edit those specs to make a change pass; do not edit `pkg/factory` at all.
- **Do not change `default:"unknown"`.** It is a Prometheus grouping label; changing it would alter every existing `task_type` series and is not needed to fail fast.
- **Do not edit the external agent library.** No `replace`, no fork, no `vendor/` — `docs/dod.md` forbids `replace` and `exclude` directives.
- **Do not touch the service-agent path.** The guard sits after the `AGENT_TYPE=service` early return, so a service agent is unaffected. Do not move it above that branch.
- **Errors use `github.com/bborbe/errors`** — never `fmt.Errorf`, never a bare `return err`, never `context.Background()` in non-test code.
- **Do not add a new dependency.** `agentlib` and `errors` are already imported in `main.go`.
- **Existing tests must still pass.** `ROOTDIR=/workspace make precommit` must exit 0. `make precommit` runs `format`, `generate` and `addlicense`, which can rewrite `.go` files with no source change behind them — leave such a rewrite alone; do not revert it by hand and do not treat it as this prompt's work.
- **Out of scope, and not a deliverable:** auditing or editing task/executor (a different repository, absent from this checkout). The root cause is a publisher that dispatched a job without a task type; this prompt only makes that symptom diagnosable here. Do not invent cross-repo changes.
- **No new package, binary, symlink, alias or credential.**
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does. Absence checks are written as `! grep -q`, which prints nothing and exits 0.

1. `grep -n 'func validateTaskType' main.go` — prints one line (the helper exists).
2. `grep -n 'validateTaskType(ctx, agentlib.TaskType(a.TaskType))' main.go` — prints exactly one line (the guard is wired into `Run`).
3. `awk '/factory.CreateAgentProvider/{c=NR} /validateTaskType\(ctx, agentlib.TaskType\(a.TaskType\)\)/{v=NR} END{exit !(v && c && v<c)}' main.go` — exits 0 (the guard runs before the provider is constructed).
4. `grep -c 'TASK_TYPE is empty' main.go` — prints `1`; `grep -c 'TASK_TYPE is unset' main.go` — prints `1`.
5. `grep -c 'default:"unknown"' main.go` — prints `1` (the flag default is unchanged).
6. `! grep -q 'agentlib.TaskTypeLLM' main.go` — exits 0 (no fallback to the LLM agent was added in `main.go`).
7. `grep -c 'agentlib.TaskTypeLLM:' pkg/factory/factory.go` — prints `1` (the dispatch table is untouched).
8. `go test -mod=mod -count=1 .` — exits 0 (the new `TASK_TYPE parsing` and `validateTaskType` specs pass, together with the existing main-package specs).
9. `go test -mod=mod -count=1 ./pkg/factory/...` — exits 0 (the pinned error contract still holds).
10. `grep -c 'TASK_TYPE parsing' main_internal_test.go` — prints at least `1`; `grep -c 'validateTaskType' main_internal_test.go` — prints at least `1`.
11. `grep -q '| `TASK_TYPE` |' README.md` — exits 0 (the env-var row was added).
12. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
13. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
14. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (one `fix:` bullet).
15. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' main.go main_internal_test.go README.md CHANGELOG.md` — exits 0 (no credential material).
16. `ROOTDIR=/workspace make test` — exits 0 (the full suite, including `cmd/run-task`, still passes).
17. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–6.
</verification>
