---
status: draft
created: "2026-11-10T18:20:00Z"
---

# Adopt CLAUDE_SESSION_ID from the pod environment

<summary>
- The agent reads `CLAUDE_SESSION_ID` from its own environment and hands it to the Claude runner as the session id to adopt.
- Unset means unchanged: a local `run-task` invocation and every existing deployment keep today's behaviour exactly.
- The value reaches both runner shapes — the one-shot runner and the session factory — because a service-mode agent uses the second.
- No new required configuration: an absent or malformed value is not an error at startup; a malformed one fails the run loudly when the CLI is spawned.
- This is the consumer half of the executor's `CLAUDE_SESSION_ID` injection; it needs `bborbe/agent` to have released `ClaudeRunnerConfig.SessionID` first.
</summary>

<objective>
Wire the executor-supplied `CLAUDE_SESSION_ID` environment variable into `ClaudeRunnerConfig.SessionID` for both the one-shot runner and the session factory, so a spawned agent's Claude CLI runs under the id the executor already wrote into the task file.
</objective>

<context>
Read `CLAUDE.md` for project conventions: single root module, `make test` and `make precommit` from the repository root, errors via `github.com/bborbe/errors`, `Do NOT commit` (dark-factory handles git).

Read `pkg/factory/factory.go` — `CreateClaudeRunner` (line ~43) and `CreateClaudeSessionFactory` (line ~76) both build a `claudelib.ClaudeRunnerConfig` from the same five parameters. They must stay in step; a field added to one and not the other makes a service-mode agent silently unforced while a task-mode agent works.

Read `main.go` — the config struct carries `arg:`/`env:` tagged fields (e.g. `ClaudeConfigDir`, `AgentDir`, `AnthropicModel`). The new field follows that shape, and the two factory calls are wired in `main.go` around lines 246-270.

Read `pkg/factory/factory_test.go` — the existing constructor specs; extend them rather than adding a file.

`github.com/bborbe/agent` must be bumped to a release that carries `ClaudeRunnerConfig.SessionID` before this compiles. If the field is not yet in the pinned version, bump the dependency first and say so in the PR body.
</context>

<requirements>
1. Add a config field `SessionID string` with `required:"false" arg:"session-id" env:"CLAUDE_SESSION_ID"` and a usage string naming the executor as the writer. Empty is valid and means "the CLI picks its own id".
2. Pass it into both `CreateClaudeRunner` and `CreateClaudeSessionFactory` as a new parameter, and set `SessionID` on the `claudelib.ClaudeRunnerConfig` in both. Keep the two constructors' parameter lists identical in shape and order.
3. Do not validate the value here. The library owns the shape check (it must reject a value that could inject a flag), and duplicating the rule in this binary would give two answers that can drift. A malformed value fails the run with the library's error.
4. Do not read the environment anywhere else, and do not fall back to a generated id: an agent that invents its own id defeats the anchor, because the executor's recorded value is what the census reads.
5. Extend the factory specs to assert both constructors carry the value through — a non-empty `SessionID` reaches the built config, and an empty one leaves it empty.
6. Add a `## Unreleased` CHANGELOG bullet in the project's existing style.
</requirements>

<verification>
- `make test` passes from the repository root.
- `make precommit` passes.
- `grep -n 'CLAUDE_SESSION_ID' main.go` shows the env tag, and `grep -n 'SessionID' pkg/factory/factory.go` shows both constructors setting it.
- With `CLAUDE_SESSION_ID` unset, a local `cmd/run-task` invocation behaves exactly as before — the argv carries no `--session-id`.
</verification>
