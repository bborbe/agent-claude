---
status: completed
spec: [006-build-target-remote-build]
summary: Guarded the upload and clean targets on BUILD_TARGET so remote builds skip local push/cleanup with a visible skip message while the local arm keeps its exact commands.
execution_id: agent-claude-build-target-exec-029-spec-006-guard-remote-upload-clean
dark-factory-version: v0.198.7
created: "2026-10-26T01:36:28Z"
queued: "2026-10-26T02:00:06Z"
started: "2026-10-26T02:09:39Z"
completed: "2026-10-26T02:14:03Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SHIP ORDER. This prompt requires prompt 2's `BUILD_TARGET ?= local` block in
   `Makefile.docker`. Requirement 1 makes that a loud precondition with a
   `not yet deployed (prompt 2)` message, so a ship-order mistake surfaces here instead of
   as an `ifeq` that silently never matches.

2. WHY THREE GUARDS, NOT TWO. The spec's AC 8 pins `ifeq ($(BUILD_TARGET),local)` at
   exactly `3` occurrences: one for `build` (prompt 2) and one each for `upload` and
   `clean` (this prompt). That count is the cheapest structural proof that the dispatch
   is applied to all three targets and to nothing else. Requirement 3 states the post-
   condition explicitly so a well-meaning agent does not also guard `apply` or `buca`.

3. WHY AN `else` BRANCH AT ALL. The spec's Desired Behavior 4 says a remote build
   "pushes and never loads", so `upload` must not run `docker push` (the Job already
   pushed) and `clean` must not run `docker rmi` / prune / `rm -rf vendor` (no local image
   exists, and the cluster clone's vendor tree is not this machine's). An empty `else`
   would work mechanically, but an echo makes `make buca BUILD_TARGET=<cluster>` readable
   and makes the skip observable in the dry run that verifies it.

4. THE ECHO TEXT MUST NOT TRIP THE OTHER ACs. `Makefile.docker` is grepped by the whole
   spec: `SERVICE` and `BRANCH` must be zero, `--pull` must not start a line, `date -u`
   must be zero, `docker buildx build` must occur once and `hack/build-cluster.sh` once.
   The echo strings below avoid every one of those.

5. LOCAL BEHAVIOUR IS FROZEN. `clean` keeps all four commands — `docker rmi … || true`,
   `docker image prune -f`, `docker builder prune --all -f`, `rm -rf vendor` — because the
   spec's Constraints say the local arm "keeps its current cleaning behaviour". The
   `|| true` guards stay: `clean` must not fail when the image is already gone.

6. `buca` IS UNCHANGED AND STILL CALLS ALL THREE. `buca: build upload clean apply` is
   what the spec's operator rung runs on both arms; on the cluster arm the two guards make
   it a build-only command, which is the intent.
-->

<summary>
- A remote build no longer tries to push from the invoking machine, because the cluster job already pushed.
- A remote build no longer tries to delete or prune a local image, because no local image exists.
- The local build path behaves exactly as it does today, including every cleanup command it runs.
- The skip is visible rather than silent: each guarded step prints why it did nothing.
- The combined publish command keeps working on both targets, doing only the steps that make sense for the chosen machine.
- Nothing else in the build definition changes.
</summary>

<objective>
Guard the `upload` and `clean` targets on `BUILD_TARGET` so that a build dispatched to a cluster skips the local push and the local image cleanup — printing why — while `BUILD_TARGET=local` keeps the exact commands it runs today.
</objective>

<context>
Read `docs/dod.md` first — it is this repository's declared `validationPrompt` and the Definition of Done you are graded against. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `validationPrompt: docs/dod.md`; the effective config resolves `autoRelease=false`, `hideGit=true` and `validationCommand: make precommit` (the first two come from the daemon's `--set` flags, not the file).

Read the spec at `specs/in-progress/006-build-target-remote-build.md` — Desired Behavior 4, Constraints, Failure Modes, and Acceptance Criteria 8 — because you have no other memory of it.

Read `Makefile.docker` in full. It is the only file this prompt changes. After prompt 2 of this spec it contains: the `DIRS +=` line; a variable block with `BUILD_TARGET ?= local` and the other `BUILD_*` / `REF` / `SOURCE_DATE_EPOCH` / `BUILD_DATE` declarations; a `build` target guarded by `ifeq ($(BUILD_TARGET),local)`; `check-version-tag`; `check-go-mod`; `upload`; `clean`; `apply`; and `buca: build upload clean apply`.

The `upload` and `clean` targets as they stand today (verbatim, and exactly what the local arm must keep):

```make
.PHONY: upload
upload:
	docker push $(DOCKER_REGISTRY)/$(IMAGE):$(VERSION)

.PHONY: clean
clean:
	docker rmi $(DOCKER_REGISTRY)/$(IMAGE):$(VERSION) || true
	docker image prune -f || true
	docker builder prune --all -f || true
	rm -rf vendor
```

Why the guards are needed: on the cluster arm the image is built inside a Job that holds the registry credential and pushes `docker.io/bborbe/agent-claude` itself. There is no image in the invoking machine's daemon, so `docker push` would fail with `No such image` and `clean` would prune an unrelated local builder cache and delete a `vendor/` tree that this arm never created. The spec records the consequence plainly: "the push and cleanup steps become no-ops on a remote target".

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no change to what the image contains; no deployment change (`apply` stays a no-op that deploys nothing); no persistent build cache; no change to the cluster scripts. In this prompt specifically: do NOT touch the `build` target or the variable block (prompt 2 owns them), do NOT touch `Dockerfile` (prompt 4), do NOT touch `CHANGELOG.md` or `docs/` (prompt 5), do NOT touch `apply`, `buca`, `check-version-tag` or `check-go-mod`.

The execution container has no Docker daemon and no cluster credentials, and its `.git` is masked. Nothing here builds, pushes or deletes an image. The checks are the spec's AC 8 grep set plus `make -n` dry runs that render each branch without executing it.

Coding guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — § "Makefile.docker build block", for the surrounding `upload`/`clean`/`buca` shape. Where it disagrees with this spec, the spec wins.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Precondition: prompt 2's dispatch must already be in place.** Run `grep -c 'BUILD_TARGET ?= local' Makefile.docker`. If it does not print `1`, STOP immediately and finish with `"status": "failed"` and the message `BUILD_TARGET dispatch not yet deployed (prompt 2 of spec 006)`. Do NOT add the variable block yourself and do NOT proceed.

2. **Guard `upload`.** Replace the `upload` target with exactly the block below. The fence is indented here only because it sits inside a numbered list — write every non-recipe line at column 1 with no leading spaces and every recipe line with a single TAB:
   ```make
   .PHONY: upload
   upload:
   ifeq ($(BUILD_TARGET),local)
   	docker push $(DOCKER_REGISTRY)/$(IMAGE):$(VERSION)
   else
   	@echo "upload: skipped — BUILD_TARGET=$(BUILD_TARGET); the cluster Job already pushed $(DOCKER_REGISTRY)/$(IMAGE):$(VERSION)"
   endif
   ```
   The `ifeq`, `else` and `endif` lines start in column 1 with no leading whitespace; recipe lines are TAB-indented. The local branch keeps the command byte-for-byte. The `else` branch only echoes — it must not run `docker push`, must not run `docker login`, and must not fail.

3. **Guard `clean`, keeping every local command.** Replace the `clean` target with exactly the block below, under the same indentation rule as requirement 2:
   ```make
   .PHONY: clean
   clean:
   ifeq ($(BUILD_TARGET),local)
   	docker rmi $(DOCKER_REGISTRY)/$(IMAGE):$(VERSION) || true
   	docker image prune -f || true
   	docker builder prune --all -f || true
   	rm -rf vendor
   else
   	@echo "clean: skipped — BUILD_TARGET=$(BUILD_TARGET); no local image to remove"
   endif
   ```
   All four local commands are retained, in this order, with their `|| true` suffixes. The `else` branch only echoes: it must not run `docker rmi`, any prune, or `rm -rf vendor` — deleting the local `vendor/` tree on a remote build would remove something that build never created.

4. **Guard nothing else.** After this change `grep -c 'ifeq ($(BUILD_TARGET),local)' Makefile.docker` must print exactly `3` — one for `build`, one for `upload`, one for `clean`. Do NOT add a fourth guard to `apply` or `buca`, and do NOT add a second `ifeq` to either target.

5. **Do not let the new text trip the rest of the spec's greps.** The file must still contain zero occurrences of `SERVICE` and zero of `BRANCH` (case-sensitive), zero of `date -u`, exactly one of `docker buildx build`, exactly one of `hack/build-cluster.sh`, exactly one of `BUILD_TARGET ?= local`, exactly one `TAG='$(VERSION)'` line, and no flag line beginning with `--pull`. The echo strings in requirements 2 and 3 are written to avoid all of these; keep them as written.

6. **Leave `apply`, `buca`, `check-version-tag` and `check-go-mod` byte-for-byte unchanged.** In particular `buca: build upload clean apply` stays on one line, and `apply` stays the recursive echo-only loop.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) `ifeq ($(BUILD_TARGET),local)` occurs exactly three times; (ii) `make -n upload BUILD_TARGET=sun` prints the skip echo and no `docker push`; (iii) `make -n clean BUILD_TARGET=sun` prints the skip echo and no `docker rmi`, no prune and no `rm -rf vendor`; (iv) `make -n upload BUILD_TARGET=local` and `make -n clean BUILD_TARGET=local` print the original commands unchanged; (v) the only file you touched is `Makefile.docker`.
</requirements>

<constraints>
- **A remote build pushes and never loads.** The Job owns the push; the invoking machine owns nothing. `upload` and `clean` must be observable no-ops when `BUILD_TARGET` is not `local`.
- **The local arm keeps its current cleaning behaviour** — `docker image prune -f`, `docker builder prune --all -f`, `rm -rf vendor`, plus the `docker rmi … || true` — so a local build cleans what it cleans today. Do not reorder, drop or strengthen any of them.
- **`clean` must not fail when the image is already gone.** Keep the `|| true` suffixes.
- **Do not add a `docker login`, a credential, or a `docker push` fallback** to either branch. The spec's Security section is explicit that the push credential exists only inside the cluster; nothing in the Makefile carries it.
- **`apply` deploys nothing.** Do not guard it and do not change it — the spec's Constraints record that `make build upload clean` and `make buca` have the same effect here.
- **No `git` command anywhere in `<verification>`.** `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT commit — dark-factory handles git.
- Do NOT run `docker`, `make build`, `make buca`, `kubectl`, `hack/*.sh` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling. Use `make -n` (dry run) only.
- Change ONLY `Makefile.docker`. Do NOT touch `Makefile`, `Makefile.precommit`, `Makefile.variables`, `Dockerfile`, `CHANGELOG.md`, `docs/`, `hack/`, `go.mod`, `go.sum` or any `.go` file.
- Existing tests must still pass.
- This prompt adds no Go code. If you find yourself tempted to write one, don't.
</constraints>

<verification>
Run each command from the repository root and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`.

A note on reading the results: absence checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero. The `make -n` commands render recipes without executing them.

1. `grep -c 'BUILD_TARGET ?= local' Makefile.docker` — prints `1` (the ship-order guard for prompt 2).
2. `grep -c 'ifeq ($(BUILD_TARGET),local)' Makefile.docker` — prints `3` (AC 8: `build`, `upload` and `clean`, and nothing else).
3. `make -n upload BUILD_TARGET=local | grep -c 'docker push docker.io/bborbe/agent-claude'` — prints `1` (the local push is unchanged; the pattern is the *expanded* command, because `make -n` prints the recipe after variable expansion, so grepping for `$(DOCKER_REGISTRY)` would never match).
4. `! make -n upload BUILD_TARGET=sun | grep -q 'docker push'` — exits 0; `make -n upload BUILD_TARGET=sun | grep -c 'upload: skipped'` — prints `1` (the remote push is a visible no-op).
5. `make -n clean BUILD_TARGET=local | grep -c 'rm -rf vendor'` — prints `1`; `make -n clean BUILD_TARGET=local | grep -c 'docker builder prune --all -f'` — prints `1`; `make -n clean BUILD_TARGET=local | grep -c 'docker image prune -f'` — prints `1`; `make -n clean BUILD_TARGET=local | grep -c 'docker rmi'` — prints `1` (all four local commands survive).
6. `! make -n clean BUILD_TARGET=sun | grep -q 'docker rmi\|prune\|rm -rf vendor'` — exits 0; `make -n clean BUILD_TARGET=sun | grep -c 'clean: skipped'` — prints `1` (the remote cleanup is a visible no-op).
7. `grep -c '^buca: build upload clean apply$' Makefile.docker` — prints `1` (the combined target is untouched and still calls all three).
8. `grep -c '^apply:$' Makefile.docker` — prints `1` (the deploy target is untouched).
9. `grep -c 'docker buildx build' Makefile.docker` — prints `1`; `grep -c 'hack/build-cluster.sh' Makefile.docker` — prints `1`; `grep -c '^build: check-version-tag check-go-mod' Makefile.docker` — prints `1` (prompt 2's build target is intact).
10. `! grep -q 'SERVICE' Makefile.docker` — exits 0; `! grep -q 'BRANCH' Makefile.docker` — exits 0; `! grep -q 'date -u' Makefile.docker` — exits 0.
11. `! grep -q -- '^[[:space:]]*--pull' Makefile.docker` — exits 0; `grep -c -- '^[[:space:]]*--no-cache' Makefile.docker` — prints `1`.
12. `! grep -qE '^[[:space:]]*docker login' Makefile.docker` — exits 0 (no credential-handling *step* was introduced). Do not use a bare `docker login` substring: the pre-existing comment above the `buca` target already contains the words "requires `docker login`", and that comment is to be left alone.
13. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command and the exit code the completion report carries. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–6.
</verification>

<success_criteria>
- `upload` runs `docker push` when `BUILD_TARGET=local` and prints a skip message (running nothing else) otherwise.
- `clean` runs `docker rmi … || true`, `docker image prune -f || true`, `docker builder prune --all -f || true` and `rm -rf vendor` when `BUILD_TARGET=local`, and prints a skip message (running nothing else) otherwise.
- `ifeq ($(BUILD_TARGET),local)` occurs exactly three times in `Makefile.docker`, for `build`, `upload` and `clean` only.
- `apply`, `buca`, `check-version-tag`, `check-go-mod` and the `build` target are byte-for-byte unchanged, and the variable block from prompt 2 is intact.
- `SERVICE`, `BRANCH`, `date -u` and a leading `--pull` still occur zero times; `docker buildx build`, `hack/build-cluster.sh`, `BUILD_TARGET ?= local` and `--no-cache` still occur once each.
- No `docker login` command and no credential appears in the Makefile.
- Only `Makefile.docker` is modified; `ROOTDIR=/workspace make precommit` exits 0 and every command in `<verification>` prints the stated result.
</success_criteria>

<completion_report>
End your run with the dark-factory completion report block:
- `"status": "success"` only if `ROOTDIR=/workspace make precommit` exited 0 and every verification command printed the stated result.
- `"status": "failed"` if any requirement is unmet (including requirement 1's missing-dispatch precondition) or if any verification command did not print the stated result — including a non-zero `make precommit`, with no exceptions and no "pre-existing failure" rationalisation.
- Include `"verification": {"command": "ROOTDIR=/workspace make precommit", "exitCode": <N>}` with the actual exit code.

Then append an `## Improvements` section, one bullet per observation, each tagged `[PROMPT]`, `[GUIDE]` or `[GLOBAL]`. If there is genuinely nothing to suggest, write `- None`.
</completion_report>
