---
status: draft
---

# Bump bborbe/agent to v0.99.0 and pass the new max sessions argument

<summary>
- The interactive service this image runs held one conversation per session id, and each held conversation is a live child process.
- A release ago it learned to drop a session that has gone a configured period without serving a turn, which bounds how many accumulate over time.
- That alone did not bound how many are held at once: a session that keeps serving turns is never idle, so many simultaneous callers still grew the container until the kernel killed it.
- The library now also has a maximum number of sessions, dropping the least recently used one to make room, and it binds that limit when a session is created rather than only on a periodic sweep.
- This repository adopts that release and states the maximum it wants.
- Nothing else about the service changes: the same session id still reaches the same conversation while it stays in use, and a session that was dropped is rebuilt on its next turn.
</summary>

<objective>
Adopt `github.com/bborbe/agent` v0.99.0, whose interactive service now bounds how many sessions it holds at once, and pass the maximum the constructors require, so the deployed `claude-interactive` container stops growing until the kernel kills it.
</objective>

<context>
This repository has no root `CLAUDE.md`. Read `docs/dod.md` — the Definition of Done the daemon grades this change against. This repo is a single Go module with one root `Makefile`; run `make` at the repo root.

Read before changing anything:
- `go.mod` — pins `github.com/bborbe/agent v0.98.0` at line 10; this is the bump.
- `main.go` — `application.newInteractiveService` calls `interactive.NewServiceWithPermissions(...)`, ending with `interactive.DefaultSessionIdleTimeout` (`permissions` is the second-to-last argument). That call is the one that breaks: v0.99.0 adds a final `maxSessions int` parameter to **both** `interactive.NewService` and `interactive.NewServiceWithPermissions`.
- `README.md` — the `### Service mode` paragraph describes the session behaviour this change extends.
- `$(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.0/interactive/session-cache.go` — read `DefaultMaxSessions` and the `newSessionCache` doc comment to see what the parameter means, what a non-positive value does, and why the default is the value it is. The constructors are in the sibling `interactive/service.go`.

Why the parameter is mandatory rather than defaulted: the right value is a function of the container's memory limit, which lives in the deployment rather than in the library, so a caller with a smaller pod must be able to state a smaller maximum without waiting for a library release. A non-positive value is normalised to `DefaultMaxSessions` with a warning rather than disabling the limit, so a mistake cannot silently restore the growth.

The release is a breaking change to the library's exported constructors — its CHANGELOG entry carries the `feat!:` prefix and says consumers adopt it when they bump.

Coding guides (read those present, at the in-container path): `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` (Unreleased placement and the required conventional prefixes), `/home/node/.claude/plugins/marketplaces/coding/docs/go-mod-dependency-fix-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md`.

Note on `make generate`: `make precommit` runs `generate`, which wipes and regenerates `mocks/`. This change alters no interface, so `mocks/` must come out byte-identical — if it does not, stop and say so rather than widening the change.
</context>

<requirements>
1. **Bump the dependency.** `go get github.com/bborbe/agent@v0.99.0`, then `go mod tidy`. `bborbe/agent` is already imported, so the tidy cannot drop it; confirm `go list -m github.com/bborbe/agent | grep -q 'v0.99.0'` exits 0 (the build graph resolves it, not merely what `go.mod` records) — the version the build graph resolves, not merely what `go.mod` records.

2. **Pass the maximum at the one call site.** In `main.go`, `application.newInteractiveService` calls `interactive.NewServiceWithPermissions(...)`. Add `interactive.DefaultMaxSessions` as the **final** argument, after `interactive.DefaultSessionIdleTimeout`. Do not introduce a CLI flag, an environment variable, or a new field on the `application` struct — the library's exported default is the value this deployment wants, and a second source for it would be one more thing to keep in step.
   No new test is required, and say so in your final message so `docs/dod.md`'s "Changes to existing code have tests covering at least the changed behavior" is answered rather than reported as a blocker: the existing spec `application.newInteractiveService` in `main_internal_test.go` builds the service through the real `interactive.NewServiceWithPermissions`, so it already traverses the new parameter's boundary, and the maximum is not exposed on the `interactive.Service` interface to assert against.

3. **Update the README.** The `### Service mode` paragraph describes the session behaviour. Extend it: the service now holds at most `interactive.DefaultMaxSessions` conversations at once and drops the least recently used one to make room, so a caller that returns after its conversation was dropped gets a working session that has started over rather than the previous conversation. `docs/dod.md` grades this ("README.md is updated if the change affects usage").
   Apart from this and the CHANGELOG, change nothing else. Do not touch the routes, the auth, the Agent Card, the permission registry, or the readiness probe.

4. **Add a `## Unreleased` section** directly above the first `## ` heading after the preamble (today that is `## v0.10.0`; there is no `## Unreleased` at present) and put one bullet under it, prefixed **`feat:`** — this adds bounded-memory behaviour and changes a caller-visible contract, which is what the repo's existing `feat:` entries record. State that the bump adopts the library's session size limit, that the service now passes `interactive.DefaultMaxSessions` as the maximum, and what that means operationally: the service holds at most that many conversations at once and closes and drops the least recently used one to make room, so a caller returning after a long pause still gets a working session rather than an error. Mention that the container's memory is now bounded by the number of sessions held at once, not only by how long they accumulate.

5. **Before finishing, re-run `<verification>` and confirm it passes; then walk each of the six `<summary>` bullets and each numbered requirement above against the change, and state in your final message which test or check covers each one — including the "no new test is required" answer from requirement 2.**
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Run `make` at the repo root.
- Do NOT change the module path, the image name, or anything in `Makefile.docker`.
- Do NOT pin a version other than `v0.99.0`, and do NOT edit `go.sum` by hand.
- Do NOT add a flag, an env var, or a struct field for the maximum.
- Leave the pre-existing `exclude` block in `go.mod` exactly as it is. `docs/dod.md` says `go.mod` should carry no `exclude` directive, but that block predates this change and is not part of it — do not remove it, and if you notice the criterion, name it as pre-existing rather than reporting it as a blocker.
</constraints>

<verification>
Run `ROOTDIR=/workspace make precommit` at the repo root — it must pass.

Then confirm the adoption landed:

```
go list -m github.com/bborbe/agent | grep -q 'v0.99.0'
grep -q 'DefaultMaxSessions' main.go
grep -q 'DefaultMaxSessions' CHANGELOG.md
grep -q 'DefaultMaxSessions' README.md
! grep -q 'github.com/bborbe/agent v0.98.0' go.mod
awk '/^## /{sec=$0} /DefaultMaxSessions/{print "sits under: " sec}' CHANGELOG.md   # prints: sits under: ## Unreleased
```

Each command must exit 0, and the `awk` must print `sits under: ## Unreleased` — that is the changelog fold guard, and it is the one check whose *output* matters rather than its exit code.

All the `grep` checks except the `!`-prefixed one are presence assertions, so `grep -q` is the right form: it exits 0 when the string is found and 1 when it is not. The `!`-prefixed line is the absence assertion, written that way on purpose — never `grep -c … must print 0`, because `grep -c` exits 1 on a zero count and would fail the step it was meant to pass.
</verification>
