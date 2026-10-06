---
status: completed
summary: Bumped github.com/bborbe/agent to v0.97.0 and passed an operator-configured A2A_AGENT_NAME into the Agent Card via interactive.CardConfig, with a fail-loud guard in runService, updated tests, README and CHANGELOG
execution_id: agent-claude-cardname-exec-008-bump-agent-a2a-card-name
dark-factory-version: dev
created: "2026-10-06T08:48:09Z"
queued: "2026-10-06T08:48:09Z"
started: "2026-10-06T08:51:58Z"
completed: "2026-10-06T08:59:02Z"
---

<summary>
- A service agent's A2A Agent Card now advertises the deployed agent's own name (for example `claude-interactive`) instead of the shared library default `interactive`
- The name comes from a new setting, `A2A_AGENT_NAME`, supplied alongside the existing `A2A_PUBLIC_URL`
- A service agent refuses to start without that name, exactly as it already refuses without the public address, so a misconfigured pod fails loudly instead of advertising a generic name
- The shared library is bumped to the release that lets the caller choose the card name
- Task-routed (non-service) runs are unchanged and need neither setting
- README, CHANGELOG and tests are updated to match
</summary>

<objective>
Bump `github.com/bborbe/agent` from v0.96.0 to v0.97.0 and pass an operator-configured agent name into the Agent Card, so `GET /.well-known/agent-card.json` on a deployed service agent returns `"name": "<A2A_AGENT_NAME>"`.
</objective>

<context>
This repo has no root `CLAUDE.md`. Read `docs/dod.md` — the Definition of Done you are graded against. Its "no `exclude`/`replace` in go.mod" rule is about adding directives; the pre-existing `exclude` block predates this prompt, stays untouched, and is NOT a blocker to report.

Current state (verified at `v0.8.0`, commit `9fb6f00`):

- `go.mod:10` — `github.com/bborbe/agent v0.96.0`. `go.mod` has a pre-existing `exclude (` block at line 119; leave it untouched and add no `replace` directive.
- `main.go:124` — `A2APublicURL string` field with `env:"A2A_PUBLIC_URL"` on the `application` struct, directly after `InteractiveAuthToken`.
- `main.go:252-263` (inside `runService`) — guards for `INTERACTIVE_AUTH_TOKEN` then `A2A_PUBLIC_URL`, each `errors.Errorf(ctx, "<VAR> is required for a service agent; ...")`; then a `glog.V(2).Infof` startup line.
- `main.go:311-328` — `newInteractiveService` calls `interactive.NewServiceWithPermissions(sessions, a.Listen, a.ProviderBaseURL, registry, interactive.NewAuthToken(a.InteractiveAuthToken), a.A2APublicURL, permissions)`.
- `main_internal_test.go` — binding spec for `A2A_PUBLIC_URL` (~line 88), the start spec building an `application` with `A2APublicURL` (~line 145), the missing-URL guard spec run in a goroutine with `Eventually` (~line 171), and the Agent Card contract spec `application.newInteractiveService` (~line 192) that serves the real card via `service.Handler()` + `httptest`.
- `README.md` — Service mode paragraph (~lines 19-29) and env table row for `A2A_PUBLIC_URL` (~line 51).
- `CHANGELOG.md` — newest heading `## v0.8.0`; add `## Unreleased` above it.

The library change in v0.97.0 (`bborbe/agent` PR #93): both constructors' `publicURL string` parameter is replaced, in the same position, by `card interactive.CardConfig` with fields `Name string` and `PublicURL string`; an empty `Name` makes the card advertise `interactive`. Confirm the exact signature from the module cache after bumping: `$(go env GOMODCACHE)/github.com/bborbe/agent@v0.97.0/interactive/service.go` and `.../interactive/a2a.go`.

Coding guides (read those present): `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`, `go-error-wrapping-guide.md`, `go-testing-guide.md`.
</context>

<requirements>
1. `go get github.com/bborbe/agent@v0.97.0`, then `go mod tidy`. The `exclude` block stays; no `replace`.

2. Add to `application`, directly after `A2APublicURL`:
   ```go
   // A2AAgentName names this deployed agent in its A2A Agent Card. Required for a service
   // agent; enforced in runService rather than by a required tag, for the same reason as
   // A2APublicURL.
   A2AAgentName string `required:"false" arg:"a2a-agent-name" env:"A2A_AGENT_NAME" usage:"Name the A2A Agent Card advertises for this agent (service agents only)"`
   ```

3. In `runService`, directly after the `A2A_PUBLIC_URL` guard, add a guard returning `errors.Errorf(ctx, "A2A_AGENT_NAME is required for a service agent; the Agent Card must name the deployed agent")` when `a.A2AAgentName == ""`. Add the name to the `glog.V(2).Infof` startup line as `(agent %s, endpoint %s)`.

4. In `newInteractiveService`, pass `interactive.CardConfig{Name: a.A2AAgentName, PublicURL: a.A2APublicURL}` in place of `a.A2APublicURL`. Update its doc comment: the card's name and URL now travel in one struct, and the contract spec checks both are advertised.

5. Tests in `main_internal_test.go`:
   - a binding spec for `A2A_AGENT_NAME` mirroring the `A2A_PUBLIC_URL` one, setting and asserting the value `"test-agent-name"` (not `"claude-interactive"` — that literal is reserved for the Agent Card name assertion so verification can tell the two apart);
   - the existing start spec sets `A2AAgentName: "claude-interactive"`;
   - the existing missing-URL spec also sets `A2AAgentName`, so it isolates the URL guard regardless of guard order;
   - a new missing-name spec (goroutine + `Eventually`, same shape as the missing-URL one) with token and URL set and the name empty, asserting `MatchError(ContainSubstring("A2A_AGENT_NAME"))`;
   - the Agent Card contract spec sets `A2AAgentName: "claude-interactive"` and additionally decodes the JSON body and asserts `name` equals `"claude-interactive"` (keep the existing URL / provider / `127.0.0.1` assertions), and rewrite the comment above that `Describe`: the public URL now travels inside `interactive.CardConfig`, so the spec guards which values fill the card's `Name` and `PublicURL` fields (e.g. `PublicURL: a.Listen`), not positional string order.

6. `README.md`: in the Service mode paragraph add that `A2A_AGENT_NAME` names the agent in the Agent Card and that a service pod with no `A2A_AGENT_NAME` also fails to start; add an env table row after `A2A_PUBLIC_URL`: `` | `A2A_AGENT_NAME` | yes (when `AGENT_TYPE=service`) | — | Name the A2A Agent Card advertises for this agent | ``.

7. `CHANGELOG.md`: `## Unreleased` above `## v0.8.0` with one `feat:` bullet — bump `github.com/bborbe/agent` to v0.97.0 (also pulls in the v0.96.1 result-deliverer fix that keeps an in-process-advanced phase across saves); the Agent Card names the deployed agent from `A2A_AGENT_NAME`, and a service agent refuses to start without it.
</requirements>

<constraints>
- Read the name only through the `application` struct tag — no `os.Getenv`, no new helper.
- Do not use `required:"true"`; the task-routed path must not need either A2A setting.
- Do not change the Agent Card contents beyond the name, and do not touch any Kubernetes manifest (they live in another repo).
- No `fmt.Errorf`; no `context.Background()` in non-test code.
- Do not commit; dark-factory handles git.
</constraints>

<verification>
```bash
ROOTDIR=/workspace make precommit
grep -n 'github.com/bborbe/agent v0.97.0' go.mod                  # one match
! grep -q '^replace' go.mod && echo no-replace                       # prints no-replace
grep -c '^exclude' go.mod                                            # prints 1
grep -n 'env:"A2A_AGENT_NAME"' main.go                               # one match
grep -n 'A2A_AGENT_NAME is required for a service agent' main.go     # one match
grep -n 'interactive.CardConfig{' main.go                            # one match
grep -c 'Equal("claude-interactive")' main_internal_test.go          # prints 1 (Agent Card name assertion only)
grep -n 'Equal("test-agent-name")' main_internal_test.go             # one match (binding spec)
grep -n 'ContainSubstring("A2A_AGENT_NAME")' main_internal_test.go   # one match (missing-name spec)
grep -c 'A2A_AGENT_NAME' README.md                                   # >= 2
grep -c 'A2A_AGENT_NAME' CHANGELOG.md                                # >= 1
awk '/^## /{sec=$0} /A2A_AGENT_NAME/{print "sits under: " sec}' CHANGELOG.md   # at least one line; all: ## Unreleased
```
</verification>
