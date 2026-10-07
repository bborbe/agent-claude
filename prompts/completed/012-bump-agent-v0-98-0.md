---
status: completed
summary: Bumped github.com/bborbe/agent to v0.98.0 and passed interactive.DefaultSessionIdleTimeout (15 minutes) as the final argument to interactive.NewServiceWithPermissions in main.go, with README and CHANGELOG updated for the idle-session eviction.
execution_id: agent-claude-session-idle-eviction-exec-012-bump-agent-v0-98-0
dark-factory-version: v0.196.0
created: "2026-10-07T11:42:58Z"
queued: "2026-10-07T11:42:58Z"
started: "2026-10-07T11:43:42Z"
completed: "2026-10-07T11:50:10Z"
---

# Bump bborbe/agent to v0.98.0 and pass the new session idle timeout

<summary>
- The interactive service this image runs holds one conversation per session id, and until now it held every one of them for the process's whole life.
- Each held conversation is a live child process, so the container's memory grew with the number of distinct sessions it had ever served.
- Measured on the deployed service: memory was linear in session count at roughly 88 MiB per session, and about eleven sessions reached the container's 1 GiB limit, where the kernel killed it.
- The library now closes and drops a session once it has gone a configured period without serving a turn, so the container's memory stays bounded instead of growing until the kernel kills it.
- This repository adopts that release and states the idle period it wants.
- Nothing else about the service changes: the same session id still reaches the same conversation while it stays in use, and a caller returning after a long pause gets a working session that has simply started over.
</summary>

<objective>
Adopt `github.com/bborbe/agent` v0.98.0, whose interactive service now evicts idle sessions, and pass the session idle period the constructors require, so the deployed `claude-interactive` container's memory stays bounded instead of growing until the kernel kills it.
</objective>

<context>
This repository has no root `CLAUDE.md`. Read `docs/dod.md` — the Definition of Done the daemon grades this change against. This repo is a single Go module with one root `Makefile`; run `make` at the repo root.

Read before changing anything:
- `go.mod` — pins `github.com/bborbe/agent v0.97.1` at line 10; this is the bump.
- `main.go` — `application.newInteractiveService` (declared at line 327) calls `interactive.NewServiceWithPermissions(...)` at line 332. That call is the one that breaks: v0.98.0 adds a final `sessionIdleTimeout time.Duration` parameter to **both** `interactive.NewService` and `interactive.NewServiceWithPermissions`.
- `README.md` — the `### Service mode` paragraph describes the session behaviour this change alters.
- `$(go env GOMODCACHE)/github.com/bborbe/agent@v0.98.0/interactive/session-cache.go` — read `DefaultSessionIdleTimeout` (line 24, `15 * time.Minute`) and the `newSessionCache` doc comment to see what the parameter means and what a non-positive value does. The constructors themselves are in the sibling `interactive/service.go` (lines 63 and 98).

Why the parameter is mandatory rather than defaulted: a caller that never states a period would keep the unbounded cache the release exists to bound. The library normalises a non-positive value to `DefaultSessionIdleTimeout` and logs a warning rather than disabling eviction, so a mistake cannot silently restore the growth.

The release is a breaking change to the library's exported constructors — its CHANGELOG entry carries the `feat!:` prefix and says consumers adopt it when they bump.

Coding guides (read those present, at the in-container path): `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` (Unreleased placement and the required conventional prefixes), `/home/node/.claude/plugins/marketplaces/coding/docs/go-mod-dependency-fix-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md`.
</context>

<requirements>
1. **Bump the dependency.** `go get github.com/bborbe/agent@v0.98.0`, then `go mod tidy`. `bborbe/agent` is already imported, so the tidy cannot drop it; confirm `go list -m github.com/bborbe/agent` prints `github.com/bborbe/agent v0.98.0` — the version the build graph resolves, not merely what `go.mod` records.

2. **Pass the idle period at the one call site.** In `main.go`, `application.newInteractiveService` calls `interactive.NewServiceWithPermissions(...)`. Add `interactive.DefaultSessionIdleTimeout` as the **final** argument, after `permissions`. Do not introduce a CLI flag, an environment variable, or a new field on the `application` struct — the library's exported default is the value this deployment wants, and a second source for it would be one more thing to keep in step.
   No new test is required, and say so in your final message so `docs/dod.md`'s "Changes to existing code have tests covering at least the changed behavior" is answered rather than reported as a blocker: the existing spec `application.newInteractiveService` in `main_internal_test.go` builds the service through the real `interactive.NewServiceWithPermissions`, so it already traverses the new parameter's boundary, and the idle period is not exposed on the `interactive.Service` interface to assert against.

3. **Update the README.** The `### Service mode` paragraph claims the service holds "one conversation per session id across requests". That is no longer unconditionally true: a session that goes the configured period (15 minutes) without serving a turn has its conversation closed and rebuilt on its next turn, so a caller returning after a long pause gets a working session that has started over rather than the previous conversation. Correct that paragraph. `docs/dod.md` grades this ("README.md is updated if the change affects usage").
   Apart from this and the CHANGELOG, change nothing else. Do not touch the routes, the auth, the Agent Card, the permission registry, or the readiness probe.

4. **Add a `## Unreleased` section** directly above the first `## ` heading after the preamble (today that is `## v0.9.3`; there is no `## Unreleased` at present) and put one bullet under it, prefixed **`feat:`** — this adds bounded-memory behaviour and changes a caller-visible contract, which is what the repo's existing `feat:` entries record. State that the bump adopts the library's idle-session eviction, that the service now passes `interactive.DefaultSessionIdleTimeout` (15 minutes) as the idle period, and what that means operationally: a session that has gone fifteen minutes without serving a turn has its conversation closed and is rebuilt on its next turn, so a caller returning after a long pause still gets a working session rather than an error. Mention that the container's memory is now bounded rather than growing with every distinct session id it has served.

5. **Before finishing, re-run `<verification>` and confirm it passes; then walk each of the six `<summary>` bullets and each numbered requirement above against the change, and state in your final message which test or check covers each one — including the "no new test is required" answer from requirement 2.**
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Run `make` at the repo root.
- Do NOT change the module path, the image name, or anything in `Makefile.docker`.
- Do NOT pin a version other than `v0.98.0`, and do NOT edit `go.sum` by hand.
- Do NOT add a flag, an env var, or a struct field for the idle period.
</constraints>

<verification>
Run `ROOTDIR=/workspace make precommit` at the repo root — it must pass.

Then confirm the adoption landed:

```
go list -m github.com/bborbe/agent          # prints github.com/bborbe/agent v0.98.0
grep -q 'DefaultSessionIdleTimeout' main.go
grep -q 'DefaultSessionIdleTimeout' CHANGELOG.md
grep -q 'DefaultSessionIdleTimeout' README.md
! grep -q 'github.com/bborbe/agent v0.97.1' go.mod
awk '/^## /{sec=$0} /v0.98.0/{print "sits under: " sec}' CHANGELOG.md   # prints: sits under: ## Unreleased
```

Each command must exit 0, and the `awk` must print `sits under: ## Unreleased` — that is the changelog fold guard, and it is the one check whose *output* matters rather than its exit code.

All the `grep` checks except the `!`-prefixed one are presence assertions, so `grep -q` is the right form: it exits 0 when the string is found and 1 when it is not. The `!`-prefixed line is the absence assertion, written that way on purpose — never `grep -c … must print 0`, because `grep -c` exits 1 on a zero count and would fail the step it was meant to pass.
</verification>
