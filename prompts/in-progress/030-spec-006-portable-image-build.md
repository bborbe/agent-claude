---
status: approved
spec: [006-build-target-remote-build]
created: "2026-10-26T01:36:28Z"
queued: "2026-10-26T02:00:06Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. TWO LINES, NOT A REWRITE. The spec's Desired Behavior 6 asks for two properties: the
   build must work without a vendored tree, and the base-image default must be portable.
   Both are one-line changes in `Dockerfile` — dropping `-mod=vendor` from the `go build`
   and changing the `DOCKER_REGISTRY` default. Everything else in the Dockerfile is out of
   scope, and requirement 3 lists the invariants so a large comment block cannot be
   "tidied" along the way.

2. WHY DROPPING THE FLAG IS THE WHOLE FIX. With no `-mod` flag, Go's own default applies:
   `vendor/` is used when it exists and `go.mod`'s go directive is >= 1.14 (which it is),
   and the module cache is used otherwise. That is exactly the behaviour Desired Behavior 6
   describes — vendored when present, module cache when absent — so adding `-mod=mod`
   would be wrong: it would force the module cache even in a repo where `vendor/` is
   present, breaking the local arm that still runs `go mod vendor` via `check-go-mod`.

3. THE COMMENT MUST NOT SAY `mod=vendor`. AC 9 pins `grep -c 'mod=vendor' Dockerfile` at
   `0`, so a why-comment that quotes the removed flag would fail the AC. Requirement 2's
   comment says "module cache" and "vendor/", never the flag.

4. WHY `docker.io` IS THE DEFAULT. The cluster clone resolves base images anonymously from
   the public registry; the machine-specific mirror host is not reachable from there and is
   not reachable from every laptop either. `docker.io` is what `Makefile` already sets for
   the output tag (`DOCKER_REGISTRY ?= docker.io`), so the base images and the output tag
   now agree by default instead of by override.

5. THE FINAL `FROM alpine` IS DELIBERATELY LEFT ALONE. It is unqualified and therefore a
   pre-existing reproducibility defect; the spec's Non-goals record it as recorded, not
   fixed. Do not qualify it, do not pin it, do not add a digest.

6. CI NEVER BUILDS THE IMAGE. `.github/workflows/ci.yml` runs `make precommit` only, so a
   Dockerfile change is not proven by a green PR and this container has no Docker socket.
   The checks here are text assertions against `Dockerfile`; the actual build stays on the
   spec's operator rung.
-->

<summary>
- The image now builds in a clone that carries no vendored dependency tree, which is the state of every cluster clone.
- A clone that does carry a vendored tree still uses it, so local builds are unchanged.
- The image's base images now default to the public registry instead of a machine-specific mirror host, so the build no longer depends on one network.
- The base-image default now matches the default the repository already uses for the output image tag.
- Nothing else about the image changes: the same stages, the same packages, the same tools, the same entrypoint.
</summary>

<objective>
Make the image build portable: let the `go build` resolve modules from the module cache when `vendor/` is absent (and still use `vendor/` when present) by dropping the explicit vendor flag, and default `DOCKER_REGISTRY` to `docker.io` instead of a machine-specific mirror host.
</objective>

<context>
Read `docs/dod.md` first — it is this repository's declared `validationPrompt` and the Definition of Done you are graded against. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `validationPrompt: docs/dod.md`; the effective config resolves `autoRelease=false`, `hideGit=true` and `validationCommand: make precommit` (the first two come from the daemon's `--set` flags, not the file).

Read the spec at `specs/in-progress/006-build-target-remote-build.md` — Desired Behavior 6, Constraints, Non-goals, and Acceptance Criterion 9 — because you have no other memory of it.

Read `Dockerfile` in full, 204 lines. It is the only file this prompt changes. The two lines to change, quoted verbatim from the working tree:

- Line 1: `ARG DOCKER_REGISTRY=docker.prod.nuke.benjamin-borbe.de:443`
- Line 8: `RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=vendor -ldflags "-s" -a -installsuffix cgo -o /main`

Everything else in the file stays as it is. The invariants the verification reads back:

- `FROM ${DOCKER_REGISTRY}/golang:1.27.3 AS build` (line 2), `FROM ${DOCKER_REGISTRY}/alpine:3.23 AS alpine` (line 11), `FROM alpine` (line 39).
- `COPY . /workspace` (line 6), `WORKDIR /workspace` (line 7), `CMD ["/bin/bash"]` (line 9).
- The `apk --no-cache add ca-certificates curl bash nodejs npm python3 git openssl jq make util-linux gcompat` line (line 33) and its `npm install -g … @anthropic-ai/claude-code@2.1.286` continuation.
- The trivy, gh, golangci-lint/govulncheck/osv-scanner and Go-toolchain blocks (lines 100–196), each with its `--version` assertion.
- `ENV BUILD_GIT_VERSION=${BUILD_GIT_VERSION}`, `ENV BUILD_GIT_COMMIT=${BUILD_GIT_COMMIT}`, `ENV BUILD_DATE=${BUILD_DATE}` (lines 198–200) and `ENTRYPOINT ["/usr/local/bin/entrypoint.sh", "-v=2"]` (line 204).

Why this change: `make check-go-mod` still runs `go mod vendor` on the local arm, so a local build has a `vendor/` tree and keeps using it. The cluster clone has no `vendor/` — the spec's Failure Modes row says "The Dockerfile's `go build` downloads modules instead of failing" is the behaviour this change establishes — and the Job has network access to the Go module proxy. The base-registry default changes because the cluster resolves `docker.io` anonymously while the mirror host is a machine-specific address; `Makefile` already defaults `DOCKER_REGISTRY ?= docker.io` for the output tag, so this makes the two defaults agree.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no change to what the image contains — no new package, no version bump, no Dockerfile stage change beyond the module-resolution flag and the base-registry default; no base-image pinning (the final stage's unqualified `FROM alpine` is a pre-existing defect, recorded and not fixed); no release. In this prompt specifically: do NOT touch `Makefile.docker` (prompts 2 and 3 own it), do NOT touch `CHANGELOG.md` or `docs/` (prompt 5), do NOT touch `hack/` (prompt 1), do NOT add an `ARG SOURCE_DATE_EPOCH` (the spec's Constraints say BuildKit reads it from the build-arg set and needs no `ARG`).

The execution container has no Docker daemon, so the image cannot be built here, and `.github/workflows/ci.yml` runs `make precommit` only — CI does not build it either. The checks are text assertions against `Dockerfile` plus `make precommit`; the build stays on the spec's operator rung.

Coding guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — § "The `DOCKER_REGISTRY` arg" and § "Canonical shape". Note that its "Quant-cluster services hardcode the mirror" advice is the state this change reverses for this repo, per the spec.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Change the base-registry default to the public registry.** Replace line 1:
   ```dockerfile
   ARG DOCKER_REGISTRY=docker.prod.nuke.benjamin-borbe.de:443
   ```
   with:
   ```dockerfile
   ARG DOCKER_REGISTRY=docker.io
   ```
   It must be exactly that line — no trailing comment, no port, no other host. Do not add a second `ARG DOCKER_REGISTRY` anywhere, and do not change the two `FROM ${DOCKER_REGISTRY}/…` lines that consume it.

2. **Drop the explicit vendor flag from the build, so Go's own default applies.** Replace line 8:
   ```dockerfile
   RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=vendor -ldflags "-s" -a -installsuffix cgo -o /main
   ```
   with:
   ```dockerfile
   # With no explicit module flag, Go uses `vendor/` when it is present and the module
   # cache when it is not — which is the state of every cluster clone. `make check-go-mod`
   # still vendors the local arm's tree, so a local build keeps using it.
   RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s" -a -installsuffix cgo -o /main
   ```
   The remaining flags are unchanged and in the same order. The comment must NOT contain the string `mod=vendor` — the AC pins that string at zero occurrences in the whole file. Do not add `-mod=mod`: that would force the module cache even when `vendor/` is present and would break the local arm.

3. **Change nothing else.** Do NOT touch any other line: not the `FROM` lines, not the `COPY . /workspace`, not `WORKDIR`, not `CMD`, not the `apk --no-cache add` line or its continuations, not the `npm install` pin, not the trivy/gh/toolchain blocks or their `--version` assertions, not the `COPY --from=build …` lines, not the `git config --system` block, not the `ENV` lines, not the `ENTRYPOINT`. Do NOT add a package, do NOT change a version pin, do NOT add an `ARG`, and do NOT add or remove a stage. The final stage's unqualified `FROM alpine` stays exactly as it is.

4. **Post-change directive counts must be unchanged from today.** `FROM` = 3, `RUN` = 10, `ENV` = 7, `COPY` = 9, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1 — verified by the commands in `<verification>`. This is the structural proof that only line content changed, not file shape. (These are the counts in the working tree today; they are NOT the counts the older `019-spec-004-install-and-wire-credential-helper.md` prompt quotes, because the image gained the `gh` and Go-toolchain blocks since then.)

5. **Do not touch anything outside `Dockerfile`.** `Makefile.docker` still passes `--build-arg DOCKER_REGISTRY=$(DOCKER_REGISTRY)` from `Makefile`'s `DOCKER_REGISTRY ?= docker.io`, so the two defaults now agree; there is nothing to change there. Do NOT touch `docs/` (prompt 5 brings the docs in line with this default), `CHANGELOG.md` (prompt 5), `hack/` (prompt 1), `go.mod`, `go.sum`, `Makefile*` or any `.go` file.

6. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) line 1 is exactly `ARG DOCKER_REGISTRY=docker.io`; (ii) the `go build` line carries no `-mod` flag; (iii) `mod=vendor` occurs zero times and the mirror host occurs zero times in `Dockerfile`; (iv) the directive counts match requirement 4; (v) the only file you touched is `Dockerfile`.
</requirements>

<constraints>
- **The image stops requiring a vendored tree.** A cluster clone has no `vendor/`; the `go build` must resolve modules from the module cache there. A clone that does have `vendor/` must keep using it — that is why the flag is dropped rather than replaced with `-mod=mod`.
- **The base-image default is `docker.io`.** Do not keep the mirror host as a fallback, do not add a second default, and do not change the output tag logic — `Makefile` already defaults `DOCKER_REGISTRY ?= docker.io`.
- **Not a change to what the image contains.** No new package, no version bump, no stage change beyond the two lines above. The Go toolchain, trivy, `gh`, the Claude Code CLI pin and every `--version` assertion stay exactly as they are.
- **No base-image pinning.** The final stage's unqualified `FROM alpine` is a pre-existing reproducibility defect. Do not qualify it, pin it, or add a digest — the spec records it as out of scope.
- **No `ARG SOURCE_DATE_EPOCH`.** BuildKit's dockerfile frontend reads it from the build-arg set to drive the exporter's timestamp rewrite; an `ARG` is not needed and the spec says so explicitly.
- **The image build has no CI.** `.github/workflows/ci.yml` runs `make precommit` only, with no `docker build`, so a Dockerfile change is not proven by a green PR — the operator rung is the only check that the image builds. That is why the checks here are text assertions and the build probes are explicitly out of scope.
- **No `git` command anywhere in `<verification>`.** `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT commit — dark-factory handles git.
- Do NOT run `docker`, `make build`, `make buca`, `kubectl`, `hack/*.sh` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling.
- Change ONLY `Dockerfile`.
- Existing tests must still pass.
- This prompt adds no Go code. If you find yourself tempted to write one, don't.
</constraints>

<verification>
Run each command from the repository root and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`.

A note on reading the results: absence checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `grep -c '^ARG DOCKER_REGISTRY=docker.io$' Dockerfile` — prints `1` (AC 9: the portable default).
2. `! grep -q 'docker.prod.nuke.benjamin-borbe.de' Dockerfile` — exits 0 (the machine-specific mirror host is gone).
3. `! grep -q 'mod=vendor' Dockerfile` — exits 0 (AC 9: no explicit vendor flag, and no comment quoting it).
4. `! grep -q -- '-mod=' Dockerfile` — exits 0 (no replacement flag either — `-mod=mod` would break the local arm).
5. `grep -c '^RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s" -a -installsuffix cgo -o /main$' Dockerfile` — prints `1` (the build line is exactly the old line minus the vendor flag).
6. `grep -c '^FROM ${DOCKER_REGISTRY}/golang:1.27.3 AS build$' Dockerfile` — prints `1`; `grep -c '^FROM ${DOCKER_REGISTRY}/alpine:3.23 AS alpine$' Dockerfile` — prints `1`; `grep -c '^FROM alpine$' Dockerfile` — prints `1` (all three stages and their base images are unchanged).
7. `grep -c '^FROM ' Dockerfile` — prints `3`; `grep -c '^RUN ' Dockerfile` — prints `10`; `grep -c '^ENV ' Dockerfile` — prints `7`; `grep -c '^COPY ' Dockerfile` — prints `9`; `grep -c '^WORKDIR ' Dockerfile` — prints `1`; `grep -c '^CMD ' Dockerfile` — prints `1`; `grep -c '^ENTRYPOINT ' Dockerfile` — prints `1`; `grep -c '^LABEL ' Dockerfile` — prints `1` (file shape unchanged — only line content changed).
8. `grep -c 'apk --no-cache add ca-certificates curl bash nodejs npm python3 git openssl jq make util-linux gcompat' Dockerfile` — prints `1`; `grep -c '@anthropic-ai/claude-code@2.1.286' Dockerfile` — prints `1` (no package was added and the CLI pin is unchanged).
9. `grep -c '&& trivy --version$' Dockerfile` — prints `1`; `grep -c '&& gh --version$' Dockerfile` — prints `1` (the build-time execution assertions are intact; the patterns are anchored to the `&&` so the explanatory comments that also name the flags cannot change the count).
10. `grep -c '^ENTRYPOINT \["/usr/local/bin/entrypoint.sh", "-v=2"\]$' Dockerfile` — prints `1` (the entrypoint is byte-for-byte unchanged).
11. `! grep -q '^ARG SOURCE_DATE_EPOCH' Dockerfile` — exits 0 (no `ARG` was added for the build arg the exporter reads directly).
12. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' Dockerfile` — exits 0 (no credential value).
13. `grep -c 'DOCKER_REGISTRY ?= docker.io' Makefile` — prints `1` (the output-tag default now agrees with the base-image default; this file was not modified).
14. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command and the exit code the completion report carries. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–5.
</verification>

<success_criteria>
- `Dockerfile` line 1 is exactly `ARG DOCKER_REGISTRY=docker.io`, and the mirror host `docker.prod.nuke.benjamin-borbe.de` occurs zero times in the file.
- The `go build` line is `RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s" -a -installsuffix cgo -o /main` — no `-mod=vendor` and no `-mod=mod`; the string `mod=vendor` occurs zero times in the file.
- A short comment above the build line records why the flag is gone, without quoting `mod=vendor`.
- The directive counts are unchanged: `FROM` = 3, `RUN` = 10, `ENV` = 7, `COPY` = 9, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1.
- No package, version pin, stage, `ARG`, `ENV` or `ENTRYPOINT` changed; the final stage's unqualified `FROM alpine` is untouched.
- Only `Dockerfile` is modified; `ROOTDIR=/workspace make precommit` exits 0 and every command in `<verification>` prints the stated result.
</success_criteria>

<completion_report>
End your run with the dark-factory completion report block:
- `"status": "success"` only if `ROOTDIR=/workspace make precommit` exited 0 and every verification command printed the stated result.
- `"status": "failed"` if any requirement is unmet or if any verification command did not print the stated result — including a non-zero `make precommit`, with no exceptions and no "pre-existing failure" rationalisation.
- Include `"verification": {"command": "ROOTDIR=/workspace make precommit", "exitCode": <N>}` with the actual exit code.

Then append an `## Improvements` section, one bullet per observation, each tagged `[PROMPT]`, `[GUIDE]` or `[GLOBAL]`. If there is genuinely nothing to suggest, write `- None`.
</completion_report>
