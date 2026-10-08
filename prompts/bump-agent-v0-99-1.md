---
status: draft
---

# Bump bborbe/agent to v0.99.1

<summary>
- The interactive service this image runs bounds how many conversations it holds at once.
- That bound shipped in the previous release, but it was evaluated only on the service's 30-second sweep.
- So a burst of callers arriving inside one window could still grow the container past its memory limit and be killed — the case the bound exists for.
- The library's new release evaluates the same bound when a session is created as well as on the sweep.
- This repository adopts that release. Nothing in this repository's own code changes: no exported signature moved, so the call site already passes every argument the constructors take.
</summary>

<objective>
Adopt `github.com/bborbe/agent` v0.99.1, so the deployed `claude-interactive` container enforces its session bound on the allocation path and a burst of callers can no longer overshoot it between sweeps.
</objective>

<context>
Read `docs/dod.md` — the Definition of Done the daemon grades this change against. This repository is a single Go module with one root `Makefile`; run `make` at the repo root.

Read before changing anything:
- `go.mod` — pins `github.com/bborbe/agent v0.99.0` at line 10; this is the only edit.
- `main.go` — `application.newInteractiveService` calls `interactive.NewServiceWithPermissions(...)`, whose final three arguments are `permissions`, `interactive.DefaultSessionIdleTimeout` and `interactive.DefaultMaxSessions` (lines 342–344). ⚠️ **This file does not change.** v0.99.1 alters only the unexported `sessionCache` (`Get` and `enforceLimit`); no **shipped** exported signature moves, so the call site already compiles against it. Do not add, remove or reorder an argument.
- `README.md` — the `### Service mode` paragraph already states that the service holds at most `interactive.DefaultMaxSessions` (8) conversations at once and closes and drops the least recently used one to make room. ⚠️ **That sentence becomes more accurate, not less** — it is the bound the sweep enforced and the allocation path now enforces too — so the README needs no change. Say so in your final message so `docs/dod.md`'s "README.md is updated if the change affects usage" is answered rather than reported as a blocker: this change does not alter usage.
- `$(go env GOMODCACHE)/github.com/bborbe/agent@v0.99.1/interactive/session-cache.go` — read `Get` and `enforceLimit` to see the bound applied on the miss path and the `reserve` parameter that expresses "the slot this call is about to consume". `Get` now takes the request `context.Context`.

Why this release is a patch and not a breaking change: no **shipped** exported signature moved. The only signature that changed is on the unexported `sessionCache` — `Get` gained a `context.Context` and `enforceLimit` gained a `reserve int` — and those are reachable solely through the **library's** two internal handlers, `github.com/bborbe/agent`'s `interactive/prompt.go` and `interactive/a2a-handler.go`. This repository has no call site for either: it adds no routing of its own, so consumers adopt the release by bumping with no source change. (The library's `interactive/export_test.go` also changed, but that file is test-only and has no consumer.)

Coding guides (read those present, at the in-container path): `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` (Unreleased placement and the required conventional prefixes), `/home/node/.claude/plugins/marketplaces/coding/docs/go-mod-dependency-fix-guide.md`, `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md`.

Note on `make generate`: `make precommit` runs `generate`, which wipes and regenerates `mocks/`. This change alters no interface, so `mocks/` must come out byte-identical — if it does not, stop and say so rather than widening the change.
</context>

<requirements>
1. **Bump the dependency.** `go get github.com/bborbe/agent@v0.99.1`, then `go mod tidy`. `bborbe/agent` is already imported, so the tidy cannot drop it; confirm `go list -m github.com/bborbe/agent | grep -q 'v0.99.1'` exits 0 — that asserts the version the **build graph** resolves, not merely what `go.mod` records.

2. **Change nothing else in Go source.** ⚠️ `main.go` compiles unchanged, because no exported signature moved in this release. If the build fails on `main.go`, stop and report it rather than editing the call site — a failure there would mean this is not the release described above.

3. **Add a `## Unreleased` section** directly above the first `## ` heading after the preamble (today that is `## v0.11.0`; there is no `## Unreleased` at present) and put one bullet under it, prefixed **`fix:`** — this corrects a bound that was enforced too rarely, which is what the repo's existing `fix:` entries record. State that the bump adopts the library's allocation-path enforcement of the session size limit, that the limit is now evaluated when a session is created as well as on the 30-second sweep, and what that means operationally: a burst of callers arriving inside one window can no longer overshoot the maximum and grow the container until the kernel kills it. Do not claim a new user-visible behaviour — the same sessions are held and the same ones are dropped; what changed is when the bound is evaluated.

4. **Before finishing, re-run `<verification>` and confirm it passes; then walk each of the five `<summary>` bullets and each numbered requirement above against the change, and state in your final message which test or check covers each one — including the "README needs no change" and "`main.go` compiles unchanged" answers.**
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Run `make` at the repo root.
- Do NOT change the module path, the image name, or anything in `Makefile.docker`.
- Do NOT pin a version other than `v0.99.1`, and do NOT edit `go.sum` by hand.
- Do NOT touch `main.go`, `README.md`, the routes, the auth, the Agent Card, the permission registry, or the readiness probe.
- Do NOT run `go mod vendor`, and do not build with `-mod=vendor`. `make precommit`'s `ensure` target wipes `vendor/`; this repository does not vendor and must not start.
- ⚠️ **Do NOT edit any existing released CHANGELOG section.** The released `## v0.11.0` entry contains the phrase `and enforce it on the allocation path` (that exact substring, if you want to grep for it) — which overstates what v0.99.0 did, namely enforce the limit **only** on the 30-second sweep. **Leave that released text exactly as it stands.** Your new `## Unreleased` `fix:` entry is what corrects the record, and a reader sees both together. Editing a released section to change history is forbidden; a `fix:` entry beside it is the correct repair. Do not delete the v0.11.0 entry and do not reword it.
- Leave the pre-existing `exclude` block in `go.mod` (line 119) exactly as it is. `docs/dod.md` says `go.mod` should carry no `exclude` directive, but that block predates this change and is not part of it — do not remove it, and if you notice the criterion, name it as pre-existing rather than reporting it as a blocker.
</constraints>

<verification>
Run `ROOTDIR=/workspace make precommit` at the repo root — it must pass. (`Makefile.variables` defines `ROOTDIR ?= $(shell git rev-parse --show-toplevel)`, which resolves **empty** in the execution container for the reason below; setting it explicitly is not load-bearing — it is read only by the `trivy` target as a `.trivyignore` fallback, and the local `.trivyignore` branch wins — so treat it as a convenience, not as a check.)

Then confirm the adoption landed:

```
go list -m github.com/bborbe/agent | grep -q 'v0.99.1'
grep -q 'v0.99.1' CHANGELOG.md
grep -c 'github.com/bborbe/agent v0.99.1' go.sum | grep -q '^2$'
! grep -q 'github.com/bborbe/agent v0.99.0' go.mod
awk '/interactive\.NewServiceWithPermissions\(/{f=1} f&&/^[[:space:]]*permissions,$/{s=1} s==1&&/^[[:space:]]*interactive\.DefaultSessionIdleTimeout,$/{s=2} s==2&&/^[[:space:]]*interactive\.DefaultMaxSessions,$/{s=3} END{exit !(s==3)}' main.go
awk '/^## /{sec=$0} /v0.99.1/{print "sits under: " sec}' CHANGELOG.md   # prints: sits under: ## Unreleased
```

Each command must exit 0, and the `awk` must print `sits under: ## Unreleased` — that is the changelog fold guard, and it is the one check whose *output* matters rather than its exit code.

The `grep -q` checks are presence assertions: they exit 0 when the string is found and 1 when it is not. The `!`-prefixed line is an absence assertion, written that way on purpose — never `grep -c … must print 0`, because `grep -c` exits 1 on a zero count and would fail the step it was meant to pass.

⚠️ **Do not put any `git` command in this block.** `.git` is masked in the execution container (`--set hideGit=true`) **and** this repository is a linked worktree whose `.git` is a file pointing at a host-absolute path that does not exist inside the container, so every `git` command there dies with `fatal: not a git repository` and writes nothing to stdout. A `!`-negated git pipeline would therefore exit 0 whether or not the file changed — a check that cannot fail, which is worse than no check at all. The `main.go` assertion above is a filesystem check for exactly that reason: it pins the call site's ordered argument tail (`permissions` → `DefaultSessionIdleTimeout` → `DefaultMaxSessions`), which is what "`main.go` is unchanged" means here. ⚠️ It checks the **order**, not just presence, because substituting a compiling literal for one of those three arguments — for example replacing `interactive.DefaultSessionIdleTimeout` with `15 * time.Minute` — would otherwise pass every other check in this block.
</verification>
