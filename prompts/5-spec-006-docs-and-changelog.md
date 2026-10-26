---
status: draft
spec: [006-build-target-remote-build]
created: "2026-10-26T01:36:28Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SHIP ORDER. This prompt describes what prompts 2, 3 and 4 built, so requirement 1 makes
   their presence a loud precondition: `BUILD_TARGET ?= local` in `Makefile.docker`, the
   `upload`/`clean` guards, and `ARG DOCKER_REGISTRY=docker.io` in `Dockerfile`. A doc that
   promises a flag the Makefile does not have is worse than no doc.

2. THE TWO REMOVALS ARE AC-DRIVEN, NOT EDITORIAL. AC 10 pins
   `grep -c 'docker.prod.nuke.benjamin-borbe.de' docs/local-docker-run.md` and
   `grep -c 'go mod vendor' docs/local-docker-run.md` at `0`. The first sentence of that
   file's Build section currently says `vendor/` must exist first, and a code block runs
   `go mod vendor`; both are now false as a prerequisite (prompt 4 dropped the flag) and
   both must go. Note `docs/publish-image.md` also says `make build` runs `go mod vendor` —
   that statement is still TRUE (`check-go-mod` retains it) and the AC does not touch it,
   so leave it.

3. `REF=` IS GREPPED WITH THE EQUALS SIGN. AC 10 requires `grep -c 'REF='` >= 1 in
   `docs/publish-image.md`. A bare `REF` mention would not satisfy it, so the procedure must
   show an actual assignment (`REF=vX.Y.Z`, `REF=<branch>`).

4. THE CLUSTER PROCEDURE IS THE PART THE SPEC'S OPERATOR RUNG RUNS. Writing it down is the
   point of this prompt: prerequisites (kubectl + kubeconfig, crane, teamvault-cli, and
   `docker login` only for the local arm), the one-time bootstrap, the build invocation,
   the digest check, and the node-name check that a local build plus a local push cannot
   fake. The scratch-tag cleanup (`crane delete`) is included because `clean` removes only
   the local image and the spec's Security section requires the published scratch tag to be
   removed.

5. THE VERSION-LABEL DIVERGENCE IS DOCUMENTED, NOT CLOSED. The spec is explicit that a
   cluster-built image carries the Dockerfile's `dev`/`none`/`unknown` defaults while the
   local arm stamps real values, and that this is a recorded gap. The doc says so, so an
   operator does not read the label check in `docs/publish-image.md` as a cluster-arm
   failure.

6. THE CHANGELOG BULLET IS ONE `feat:` ENTRY. The capability is new (a target switch plus a
   reproducible local arm plus a portable image), so it takes the minor bump. The bullet
   must contain `BUILD_TARGET` — that is AC 11's `awk` anchor — and must not describe what
   was verified, only what was implemented.

7. `docs/dod.md` IS NOT EDITED. It is the validation prompt, not a build document; the spec
   names only `docs/local-docker-run.md` and `docs/publish-image.md`.
-->

<summary>
- The documentation now explains that a build can run on the invoking machine or in a named cluster, and how to choose.
- The local recipe no longer tells the reader to vendor the dependency tree first, because the image builds without one.
- The local recipe no longer tells the reader to override a machine-specific registry host, because the image's default is now the public registry.
- A cluster-build procedure is documented end to end: one-time credential provisioning, the build command, the digest check, and the node check that proves the build ran in the cluster.
- The procedure warns that the two arms produce different version labels, so nobody mistakes that for a failure.
- The changelog records the change as one new-capability entry.
</summary>

<objective>
Bring the two build documents in line with the new build targets — `docs/local-docker-run.md` loses its vendored-tree prerequisite and its machine-specific registry default, and `docs/publish-image.md` gains the cluster procedure — and record the change under `## Unreleased` in `CHANGELOG.md`.
</objective>

<context>
Read `docs/dod.md` first — it is this repository's declared `validationPrompt` and the Definition of Done you are graded against. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `validationPrompt: docs/dod.md`; the effective config resolves `autoRelease=false`, `hideGit=true` and `validationCommand: make precommit` (the first two come from the daemon's `--set` flags, not the file).

Read the spec at `specs/in-progress/006-build-target-remote-build.md` — Desired Behaviors 1–6, the Verification section (both rungs), Constraints, Security, and Acceptance Criteria 10 and 11 — because you have no other memory of it. The operator-executable rung of that Verification section is the procedure you are documenting; copy its commands, not a paraphrase of them.

Read these files before editing:

- `Makefile.docker` — after prompts 2 and 3 it declares `BUILD_TARGET ?= local`, `BUILD_NAMESPACE ?= build`, `BUILD_TIMEOUT ?= 3600`, `BUILD_REPO_URL ?= https://github.com/$(IMAGE).git`, `BUILD_DOCKERFILE ?= Dockerfile`, `BUILD_PUSH_SECRET ?= buildkit-push`, `REF ?=` (empty), `SOURCE_DATE_EPOCH ?= $(shell git log -1 --format=%ct)`, `BUILD_DATE ?= $(shell git log -1 --format=%cI)`; `build` dispatches on `BUILD_TARGET`; and `upload`/`clean` are guarded by `ifeq ($(BUILD_TARGET),local)`. Document what is there.
- `Dockerfile` — after prompt 4, line 1 is `ARG DOCKER_REGISTRY=docker.io` and the `go build` line carries no `-mod` flag.
- `hack/build-cluster-bootstrap.sh` and `hack/build-cluster.sh` — vendored by prompt 1. Read them so the documented commands and the documented prerequisites match what the scripts actually do: the bootstrap is idempotent and provisions the `buildkit-push` Secret; the build script requires `REF` and refuses to apply over an existing Job of the derived name.
- `docs/local-docker-run.md` — the whole file, 206 lines. The Build section is lines 25–51. Two statements in it must go (requirement 2): line 28's "`vendor/` must exist first — the Dockerfile builds with `-mod=vendor`:" with its `go mod vendor` code block (lines 31–33), and lines 46–47's "`DOCKER_REGISTRY` defaults to the internal mirror (`docker.prod.nuke.benjamin-borbe.de:443`) in the Dockerfile — override it to `docker.io` unless the laptop has mirror access."
- `docs/publish-image.md` — the whole file, 89 lines. It documents the manual local publish path; the cluster arm is added alongside it.
- `CHANGELOG.md` — line 1 is `# Changelog`; the first `## ` heading is `## v0.25.0` at line 9. There is no `## Unreleased` section today.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no change to what the image contains; no deployment change (`apply` deploys nothing, and the cluster still pulls the mirrored quant-registry image rather than docker.io — say so, do not change it); no edit to `bborbe/go-skeleton` or `hack/`; no release (verification runs on a scratch version, the release is cut by the maintainer after merge). In this prompt specifically: do NOT touch `Makefile.docker`, `Dockerfile`, `hack/`, `go.mod`, `go.sum`, any `.go` file, `README.md` or `docs/dod.md`.

The execution container has no Docker daemon, no `kubectl`, no `crane`, no `teamvault-cli` and no git repository. Nothing documented here can be executed in this container; the checks are text assertions plus `make precommit`. Do not attempt to run any documented command.

Coding guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement, the required conventional prefix, the frozen-preamble rule, and the anti-patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` — how a procedure page is structured (prerequisites, procedure, verify, related).
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Precondition: the code this prompt documents must already be in place.** Run all three:
   ```bash
   grep -c 'BUILD_TARGET ?= local' Makefile.docker
   grep -c 'ifeq ($(BUILD_TARGET),local)' Makefile.docker
   grep -c '^ARG DOCKER_REGISTRY=docker.io$' Dockerfile
   ```
   They must print `1`, `3` and `1`. If any does not, STOP immediately and finish with `"status": "failed"` and the message `build target dispatch not yet deployed (prompts 2-4 of spec 006)`. Do NOT add the code yourself and do NOT document a flag that does not exist.

2. **Rewrite the Build section of `docs/local-docker-run.md`.** In the section beginning `## Build` and ending before `## Run`:
   - Delete the second sentence of the Build section's opening paragraph — the one beginning "`vendor/` must exist first" and wrapping across lines 27–29 — together with the fenced `go mod vendor` code block at lines 31–33. Replace them with a sentence stating that no vendored tree is needed: the build resolves modules from the module cache when `vendor/` is absent and uses `vendor/` when it is present, so a fresh clone builds as-is. The file must contain zero occurrences of `go mod vendor` afterwards.
   - Keep the `docker build` example (host-architecture build for a local loop) and keep its `--build-arg DOCKER_REGISTRY=docker.io` line — it is still correct and now agrees with the default.
   - Replace the paragraph that names the internal mirror host with one stating that `DOCKER_REGISTRY` defaults to `docker.io` in the Dockerfile, so the example needs no override for a laptop without mirror access. The file must contain zero occurrences of `docker.prod.nuke.benjamin-borbe.de` afterwards.
   - Add one short paragraph naming the two build targets: `make build BUILD_TARGET=local` (the default; builds here, reproducible, and needs a `docker`-driver buildx builder for two builds to agree) and `make build BUILD_TARGET=<cluster> REF=<branch or tag>` (builds as a Job in that cluster; see `publish-image.md`). Say that a cluster build leaves no image on the invoking machine, so `upload` and `clean` do nothing on that arm. Keep the existing `ALLOW_UNTAGGED_BUILD=1 VERSION=scratch make build` example, corrected only if it now needs `BUILD_TARGET=local` to be unambiguous — it does not, because `local` is the default.
   - Leave every other section unchanged, including `## Prerequisites`, `## Run`, the env table, `## Verify`, the two-turn recipe, push notifications, the heartbeat note, `### Pick a free host port`, `## Gotchas` and `## Related`.

3. **Add a cluster procedure to `docs/publish-image.md`.** Keep the existing manual local procedure intact (the worktree recipe, `VERSION=vX.Y.Z make build upload`, the `docker manifest inspect` and label checks, the toolchain check, the "After publishing" section and "Related"). Then add a section (for example `## Build in the cluster`) that covers, in this order:
   - **Why**: the credential never sits on the invoking machine; the Job holds it, builds, and pushes. Note that a cluster build is equivalent to a local one in *where* it builds, not in the bytes it emits: the Job passes only the commit epoch, so the image carries the Dockerfile's `dev`/`none`/`unknown` defaults while a local build stamps real values. That divergence is recorded, not closed — do not present it as a defect to fix here.
   - **Prerequisites**: `kubectl` with a kubeconfig for the target cluster, `crane`, and `teamvault-cli` logged in to the account holding the registry key. `docker login` is needed only for the local arm.
   - **One-time provisioning**: `hack/build-cluster-bootstrap.sh <cluster>` — idempotent, creates the `build` namespace and the `buildkit-push` Secret. Mention that re-running it is the fix for a missing secret or for a `buildkit-push` Secret that was replaced with the reflector's `docker` secret.
   - **Build**: `ALLOW_UNTAGGED_BUILD=1 VERSION=<scratch> REF=<branch> make build BUILD_TARGET=<cluster>` for a scratch build, and `VERSION=vX.Y.Z REF=vX.Y.Z make build BUILD_TARGET=<cluster>` for a release build. State that `REF=` is required on this arm and has no default, and that `sun`, `nukedev` and `nukeprod` are aliases while any other value is a raw kubectl context name. Add one short **When it refuses** note: a Job named `build-<tag-lowercased>` that already exists makes the script refuse and print its own delete command (`kubectl -n build delete job <name>`), and a `REF` whose cluster commit differs from the local one fails the build container with both epochs named, fixed by `git fetch` and a re-run.
   - **Verify**: `crane digest docker.io/bborbe/agent-claude:<tag>` returns the digest the build printed, and `kubectl -n build get pod -l job-name=build-<tag-lowercased> -o jsonpath='{.items[0].spec.nodeName}'` names a node in the target cluster — the check a local build plus a local push cannot fake.
   - **Clean up a scratch tag**: `crane delete docker.io/bborbe/agent-claude:<scratch>`; `clean` removes only the local image, so it cannot do this.
   The file must contain at least one occurrence each of `BUILD_TARGET`, `REF=` (with the equals sign, as in `REF=<branch>`) and `build-cluster-bootstrap.sh`.

4. **Record the change in `CHANGELOG.md`.** There is no `## Unreleased` section today. Insert one directly above `## v0.25.0`, holding exactly one bullet prefixed `feat:` (this is a new capability, so it takes the minor bump). The bullet must state, in the style of the existing entries (one long sentence, backticked identifiers, em dashes): `make build` now dispatches on `BUILD_TARGET` — `local` (the default) builds on the invoking machine with `docker buildx build` and a commit-derived clock, producing the same image ID for two builds of one commit, while any other value runs the build as a Job in that cluster via the vendored `hack/` scripts, with the registry credential held only inside the pod; `upload` and `clean` are no-ops on the cluster arm because the Job already pushed and no local image exists; the image no longer requires a vendored tree and defaults its base registry to `docker.io`. The bullet MUST contain the exact token `BUILD_TARGET` (the spec's AC 11 anchor). Do not describe what was verified; describe what was implemented.

   Do not move, delete or reorder the preamble, and do not edit any existing released section.

5. **Change nothing else.** Do NOT touch `Makefile.docker`, `Dockerfile`, `hack/`, `README.md`, `docs/dod.md`, `docs/cluster-a2a-run.md`, `docs/attention-store.md`, `docs/claude-oauth-setup.md`, `docs/creating-claude-agents.md`, `go.mod`, `go.sum`, `scripts/`, `agent/` or any `.go` file. Do NOT add a doc page. Do NOT change the statement in `docs/publish-image.md` that `make build` runs `go mod vendor` — `check-go-mod` still does, so it remains true.

6. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) `docs/local-docker-run.md` contains zero `go mod vendor` and zero mirror-host occurrences while still containing its `## Prerequisites`, `## Run` and `## Verify` sections; (ii) `docs/publish-image.md` contains `BUILD_TARGET`, `REF=` and `build-cluster-bootstrap.sh` and still contains its local procedure; (iii) `CHANGELOG.md` has exactly one `## Unreleased` section directly above `## v0.25.0`, holding exactly one `feat:` bullet that contains `BUILD_TARGET`; (iv) the only files you touched are those three.
</requirements>

<constraints>
- **`hack/` is a verbatim copy of `bborbe/go-skeleton` at `18866d9d`.** Do not edit it, and do not document it as locally customised — the doc must point at the Makefile variables, never at a script edit.
- **The push credential is named, never inlined.** The doc may name `teamvault-cli`, `hack/dockerconfig.json.tpl` and the `buildkit-push` Secret; it must never show a credential value, an `auth` blob, a token or a rendered Docker config. `crane`, `kubectl` and `teamvault-cli` invocations in the doc must be names-only where they print cluster state (`-o name`, digest, node name).
- **`apply` deploys nothing, and the cluster still pulls the mirrored quant-registry image.** Say both plainly; do not write a deployment procedure and do not suggest that a cluster build changes what the cluster runs.
- **No `docker login` step on the cluster arm.** The Job holds the credential; the doc must not tell an operator to log the invoking machine in for a remote build.
- **No release.** The procedure uses a scratch version and ends by deleting the scratch tag; do not bump a version, do not create a tag, and do not describe cutting a release.
- **The changelog has exactly one bullet** under a new `## Unreleased`, above `## v0.25.0`, and no existing section is edited. Do not add a `## Unreleased` section if one already exists — merge into it, still with exactly one bullet for this change.
- **Do NOT run any documented command.** This container has no Docker daemon, no `kubectl`, no `crane`, no `teamvault-cli` and no git repository. The doc is verified by reading it and by the text assertions below.
- **No `git` command anywhere in `<verification>`.** `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT commit — dark-factory handles git.
- Do NOT run `docker`, `make build`, `make buca`, `kubectl`, `hack/*.sh` or `scripts/*.sh`.
- Change ONLY `docs/local-docker-run.md`, `docs/publish-image.md` and `CHANGELOG.md`.
- Existing tests must still pass.
- This prompt adds no Go code. If you find yourself tempted to write one, don't.
</constraints>

<verification>
Run each command from the repository root and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`.

A note on reading the results: absence checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `grep -c 'BUILD_TARGET ?= local' Makefile.docker` — prints `1`; `grep -c 'ifeq ($(BUILD_TARGET),local)' Makefile.docker` — prints `3`; `grep -c '^ARG DOCKER_REGISTRY=docker.io$' Dockerfile` — prints `1` (the ship-order guard for prompts 2–4; the doc must not describe code that is absent).
2. `! grep -q 'docker.prod.nuke.benjamin-borbe.de' docs/local-docker-run.md` — exits 0 (AC 10).
3. `! grep -q 'go mod vendor' docs/local-docker-run.md` — exits 0 (AC 10).
4. `grep -c 'docker.io' docs/local-docker-run.md` — prints at least `1` (the portable default is stated).
5. `grep -c 'BUILD_TARGET' docs/local-docker-run.md` — prints at least `1` (both arms are named).
6. `grep -c '^## Prerequisites$' docs/local-docker-run.md` — prints `1`; `grep -c '^## Build$' docs/local-docker-run.md` — prints `1`; `grep -c '^## Run$' docs/local-docker-run.md` — prints `1`; `grep -c '^## Verify$' docs/local-docker-run.md` — prints `1`; `grep -c '^## Gotchas$' docs/local-docker-run.md` — prints `1` (the file's structure survives the Build-section rewrite).
7. `grep -c 'agent-claude:local-scratch' docs/local-docker-run.md` — prints at least `1`; `grep -c 'A2A_PUBLIC_URL' docs/local-docker-run.md` — prints at least `1` (the run recipe is untouched).
8. `grep -c 'BUILD_TARGET' docs/publish-image.md` — prints at least `1` (AC 10).
9. `grep -c 'REF=' docs/publish-image.md` — prints at least `1` (AC 10; the equals sign is required).
10. `grep -c 'build-cluster-bootstrap.sh' docs/publish-image.md` — prints at least `1` (AC 10).
11. `grep -c 'docker.io/bborbe/agent-claude' docs/publish-image.md` — prints at least `1`; `grep -c 'docker manifest inspect' docs/publish-image.md` — prints at least `1` (the existing local procedure is intact).
12. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|"auth"[[:space:]]*:[[:space:]]*"[A-Za-z0-9+/=]{16,}"' docs/local-docker-run.md docs/publish-image.md CHANGELOG.md` — exits 0 (no credential value in any file this change touches).
13. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
14. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1` (AC 11).
15. `awk '/^## Unreleased/{f=1} f && /BUILD_TARGET/{print; exit}' CHANGELOG.md` — prints a line containing `BUILD_TARGET` (AC 11).
16. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (the new section carries exactly one bullet).
17. `awk '/^## /{sec=$0} /BUILD_TARGET/{print "sits under: " sec}' CHANGELOG.md | head -1` — prints `sits under: ## Unreleased` (the bullet did not fold into a released section).
18. `grep -c '^## v0.25.0' CHANGELOG.md` — prints `1` (the previously-first released heading is still present, directly below the new section).
19. `grep -c 'feat:' CHANGELOG.md` — prints at least `1`, and `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- feat:/{n++} END{print n+0}' CHANGELOG.md` — prints `1` (the one new bullet carries the `feat:` prefix, so the version bump is a minor one).
20. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command and the exit code the completion report carries. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–5.
</verification>

<success_criteria>
- `docs/local-docker-run.md` no longer contains `go mod vendor` or `docker.prod.nuke.benjamin-borbe.de`, states that no vendored tree is needed, states that the base registry defaults to `docker.io`, and names both `BUILD_TARGET=local` and `BUILD_TARGET=<cluster> REF=<ref>`; its Prerequisites, Run, Verify and Gotchas sections are intact.
- `docs/publish-image.md` keeps the existing local procedure and adds a cluster procedure covering why (credential stays in the cluster, version-label divergence is expected), prerequisites (`kubectl`, `crane`, `teamvault-cli`), one-time `hack/build-cluster-bootstrap.sh <cluster>`, the `REF=`-bearing build command with the `sun`/`nukedev`/`nukeprod` aliases, the `crane digest` and node-name checks (noting that the Job name is `build-` plus the tag lowercased and sanitised), and `crane delete` for a scratch tag.
- Neither doc shows a credential value, an `auth` blob or a rendered Docker config, and neither instructs a `docker login` for the cluster arm.
- `CHANGELOG.md` gains exactly one `## Unreleased` section directly above `## v0.25.0`, holding exactly one `feat:` bullet that contains `BUILD_TARGET`; no existing section is edited and the preamble is intact.
- Only `docs/local-docker-run.md`, `docs/publish-image.md` and `CHANGELOG.md` are modified; `ROOTDIR=/workspace make precommit` exits 0 and every command in `<verification>` prints the stated result.
</success_criteria>

<completion_report>
End your run with the dark-factory completion report block:
- `"status": "success"` only if `ROOTDIR=/workspace make precommit` exited 0 and every verification command printed the stated result.
- `"status": "failed"` if any requirement is unmet (including requirement 1's precondition) or if any verification command did not print the stated result — including a non-zero `make precommit`, with no exceptions and no "pre-existing failure" rationalisation.
- Include `"verification": {"command": "ROOTDIR=/workspace make precommit", "exitCode": <N>}` with the actual exit code.

Then append an `## Improvements` section, one bullet per observation, each tagged `[PROMPT]`, `[GUIDE]` or `[GLOBAL]`. If there is genuinely nothing to suggest, write `- None`.
</completion_report>
