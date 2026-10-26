---
status: draft
spec: [006-build-target-remote-build]
created: "2026-10-26T01:36:28Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SHIP ORDER. This prompt requires `hack/build-cluster.sh` to exist (prompt 1 of this
   spec). Requirement 1 makes that a loud precondition: if the file is absent the agent
   stops with a `not yet deployed (prompt 1)` message instead of writing a Makefile that
   calls a missing script.

2. WHY THE VARIABLE BLOCK SITS ABOVE THE TARGETS. `ifeq ($(BUILD_TARGET),local)` is a
   parse-time directive, so `BUILD_TARGET` must be assigned before the conditional is
   read. A command-line `BUILD_TARGET=sun` is set by make before parsing, so `?=` does not
   override it. The reference `Makefile.docker` in go-skeleton has the same shape
   (variables, then `ifeq` immediately after the target line), which is what makes the
   conditional-in-recipe idiom safe here.

3. `REF ?=` IS DELIBERATE, AND THE SPEC IS SLIGHTLY TENSE ABOUT IT. Desired Behavior 2
   says `REF` "is a required input on this arm and has no default", while Constraints say
   the reference's variables are "declared in full … for parity with the reference, so an
   operator can override one without editing the script". The reference declares `REF ?=`
   (empty). Resolved as: declare it empty so it is overridable and documented, and let
   `hack/build-cluster.sh` enforce presence with its own `REF is required` message. That
   satisfies both sentences and keeps the script's failure mode reachable.

4. THE `BUILD_DATE` CHANGE CONTRADICTS TWO CODING GUIDES ON PURPOSE. Both
   `go-build-args-guide.md` and `go-dockerfile-guide.md` show
   `BUILD_DATE=$$(date -u …)`. Spec AC 5 requires `grep -c 'date -u' Makefile.docker` to
   return 0 and `BUILD_DATE ?=` to return 1, because a wall-clock date breaks the
   two-build image-ID equality the spec establishes. The spec wins; the guides should be
   amended (flagged in `## Improvements` as `[GUIDE]`).

5. `--no-cache` IS RETAINED AND `--rm=true` IS DROPPED, both by spec Constraints. The
   reference buildx arm in go-skeleton carries neither, so this arm is not a literal copy
   of it — it is the reference arm plus the two deltas the spec names. `--pull` is
   dropped: a moving base image would break the image-ID equality.

6. `--output` GOES AFTER THE CONTEXT PATH, exactly as the reference does it. buildx
   accepts interspersed flags, and copying the reference's argument order keeps the
   `grep -c 'rewrite-timestamp=true,unpack=false'` anchor stable.

7. WHAT THIS PROMPT DOES NOT DO. It does not touch `upload` or `clean` (prompt 3), the
   Dockerfile (prompt 4), or the docs and changelog (prompt 5). Its verification asserts
   `ifeq ($(BUILD_TARGET),local)` occurs exactly ONCE — prompt 3 raises that to three,
   which is a clean cross-prompt postcondition.

8. OPERATOR RUNG IS NOT CARRIED. Neither the two-build image-ID equality nor a cluster
   job can run here (no Docker daemon, no kubeconfig). Those stay on the spec's
   Operator-executable rung. The structural evidence here is the AC grep set plus
   `make -n`, which renders each arm's recipe without executing it.
-->

<summary>
- One variable now selects where the image is built: the default keeps the build on the invoking machine, and any other value names a cluster where the build runs as a job.
- The invoking-machine path writes a reproducible image, so two builds of one commit produce the same image ID.
- The build clock now comes from the commit rather than the wall clock, which is what makes those two builds agree.
- The cluster path hands the reference script the repository's own image name, version, clone URL and Dockerfile path, so the same script works for this repo without being edited.
- The repository's existing variable names survive the port: the image name, the tag, and the clone URL derived from the image name.
- The build still refuses to run from the wrong commit, and still vendors the module tree first, because both prerequisites are kept.
- The upload and cleanup steps are untouched and still run locally, so nothing regresses for anyone building on their own machine.
- Nothing is built here: the checks confirm the build definition dispatches correctly, while the actual build and push remain operator steps.
</summary>

<objective>
Make `make build` dispatch on `BUILD_TARGET`: keep the default `local` arm on `docker buildx build` with a commit-derived clock, provenance/SBOM attestations disabled, and the reproducible docker exporter recipe; and add a cluster arm that passes the repository's own variables to `hack/build-cluster.sh`, so `make build BUILD_TARGET=<cluster> REF=<ref>` builds and pushes the image as a Job in that cluster.
</objective>

<context>
Read `docs/dod.md` first — it is this repository's declared `validationPrompt` and the Definition of Done you are graded against. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `validationPrompt: docs/dod.md`; the effective config resolves `autoRelease=false`, `hideGit=true` and `validationCommand: make precommit` (the first two come from the daemon's `--set` flags, not the file).

Read the spec at `specs/in-progress/006-build-target-remote-build.md` — Desired Behavior 1, 3 and 5, Constraints, Failure Modes, and Acceptance Criteria 3–7 — because you have no other memory of it.

Read these files before editing:

- `Makefile.docker` — the whole file, 70 lines. It is the only file this prompt changes. Current content, quoted: line 1 is `DIRS += $(shell find */* -maxdepth 0 -name Makefile -exec dirname "{}" \;)`; the `build` target is lines 3–17 with `build: check-version-tag check-go-mod` on line 4 and `DOCKER_BUILDKIT=1 docker build --pull --no-cache --rm=true --platform=linux/amd64 …` from line 6; `check-version-tag` is lines 19–33; `check-go-mod` is lines 35–42; `upload` is lines 44–46; `clean` is lines 48–53; `apply` is lines 55–62; `buca: build upload clean apply` is line 69.
- `Makefile` — the including file. `include Makefile.variables`, `include Makefile.precommit`, `include Makefile.docker` come first (lines 1–3), and `DOCKER_REGISTRY ?= docker.io`, `IMAGE ?= bborbe/agent-claude` and the `VERSION :=` fallback follow (lines 5–8). This ordering matters: `Makefile.docker` is parsed before `IMAGE` is assigned, so `BUILD_REPO_URL ?= https://github.com/$(IMAGE).git` must rely on `?=` being a recursively-expanded (lazy) variable — it is, and `$(IMAGE)` then resolves at recipe-expansion time.
- `hack/build-cluster.sh` — vendored by prompt 1. Read it in full. It reads exactly these environment variables: `BUILD_TARGET` (only for error text), `NAMESPACE`, `TIMEOUT`, `REPO_URL`, `REF`, `TAG`, `DOCKERFILE`, `IMAGE`, `PUSH`, `PUSH_SECRET`. It derives the Job name as `build-` plus the tag lowercased and sanitised to DNS-1123 characters, renders `hack/build-job.yaml` with `sed`, and applies it. It exits non-zero with `REF is required; pass it on the make command line` when `REF` is empty, and with a list of accepted values when the resolved kubeconfig does not know the context. **Requirement 1 makes its presence a precondition of this prompt.**
- `hack/build-job.yaml` — read it to confirm which tokens the script substitutes with `sed`. They are double-underscore tokens, not Go-template fields: `__JOB_NAME__`, `__REPO_URL__`, `__REF__`, `__TAG__`, `__DOCKERFILE__`, `__IMAGE__`, `__PUSH__`, `__PUSH_SECRET__`, `__DEADLINE_SECONDS__` and `__SOURCE_DATE_EPOCH__`. The script maps `TIMEOUT` onto `__DEADLINE_SECONDS__`, so `BUILD_TIMEOUT` is a number of seconds (the reference default is `3600`), not a duration string. This prompt does not edit the manifest; reading it is how you know the variable names and value shapes you must supply.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no change to what the image contains; no deployment change (`apply` stays a no-op); no edit to `bborbe/go-skeleton` or to the vendored `hack/` files; no persistent build cache; no version-label parity between the arms (the cluster Job passes only the commit epoch, so a cluster-built image carries the Dockerfile's `dev`/`none`/`unknown` defaults — that gap is recorded, not closed); no base-image pinning. In this prompt specifically: do NOT guard `upload` or `clean` (prompt 3 owns them), do NOT touch `Dockerfile` (prompt 4), do NOT touch `CHANGELOG.md` or `docs/` (prompt 5), do NOT touch `check-version-tag`, `check-go-mod`, `apply` or `buca`.

The execution container has no Docker daemon, no `kubectl` and no cluster credentials, and its `.git` is masked. Nothing here builds or pushes an image. The checks are the spec's AC grep set plus `make -n` dry runs that render each arm's recipe without executing it. `make -n` is safe under the masked `.git`: the `$(shell git …)` expressions in `SOURCE_DATE_EPOCH`, `BUILD_DATE` and `VERSION` expand to empty strings, which changes nothing that is asserted.

Coding guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — § "Makefile.docker build block" and § "The `DOCKER_REGISTRY` arg". Background only: where that block disagrees with this spec (it carries `--pull`, `--rm=true`, `date -u` and `-mod=vendor`), the spec wins.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-build-args-guide.md` — the three build args and why they exist. Same caveat: this spec replaces the wall-clock `BUILD_DATE` with the commit's date.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Precondition: the vendored cluster script must exist.** Run `test -f hack/build-cluster.sh && echo present`. If it does not print `present`, STOP immediately and finish with `"status": "failed"` and the message `hack/build-cluster.sh not yet deployed (prompt 1 of spec 006)`. Do NOT create the script, do NOT stub it, and do NOT write a Makefile that calls a missing file. Do NOT proceed with any other requirement.

2. **Add the variable block at the top of `Makefile.docker`, directly below the existing `DIRS +=` line and above every target.** It must read exactly as below. The fence is indented here only because it sits inside a numbered list — write every line at column 1, with no leading spaces, exactly as shown once that indentation is removed:
   ```make
   # Which machine builds the image. `local` builds here with the docker-driver
   # builder; any other value is a cluster alias (sun, nukedev, nukeprod) or a raw
   # kubectl context name, and the build runs as a Job there.
   BUILD_TARGET ?= local

   # Cluster-build inputs. The vendored cluster script defaults the namespace,
   # timeout and Dockerfile and requires the push secret; the clone URL, tag and image
   # come from this repository. They are declared here so an operator can override one
   # without editing the script.
   BUILD_NAMESPACE ?= build
   BUILD_TIMEOUT ?= 3600
   BUILD_REPO_URL ?= https://github.com/$(IMAGE).git
   BUILD_DOCKERFILE ?= Dockerfile
   BUILD_PUSH_SECRET ?= buildkit-push

   # The branch or tag the cluster clones. Empty on purpose: the cluster arm must be
   # told which commit to build, and the vendored script refuses to run without it.
   REF ?=

   # The build clock comes from the commit, not the wall clock, so two builds of one
   # commit agree. BuildKit reads SOURCE_DATE_EPOCH when it rewrites file timestamps.
   SOURCE_DATE_EPOCH ?= $(shell git log -1 --format=%ct)
   BUILD_DATE ?= $(shell git log -1 --format=%cI)
   ```
   Constraints on this block: the comment text must NOT contain the strings `SERVICE`, `BRANCH`, `date -u`, `docker buildx build`, `hack/build-cluster.sh`, `--pull`, or `mod=vendor` — the verification counts several of those across the whole file, and a comment that names one would break the count. `BUILD_TARGET ?= local` must appear exactly once in the file.

3. **Rewrite the `build` target as a two-arm dispatch, keeping its prerequisites.** Replace the existing `build` target (lines 3–17) with exactly the block below. As in requirement 2, the fence is indented for markdown only: non-recipe lines go at column 1, recipe lines start with a single TAB, and there are no other leading spaces:
   ```make
   .PHONY: build
   build: check-version-tag check-go-mod
   ifeq ($(BUILD_TARGET),local)
   	@if [ -z "$(VERSION)" ]; then echo "ERROR: no VERSION set and no git tag found — set VERSION=vX.Y.Z or create a tag" >&2; exit 1; fi
   	docker buildx build \
   	--platform=linux/amd64 \
   	--no-cache \
   	--provenance=false \
   	--sbom=false \
   	--build-arg DOCKER_REGISTRY=$(DOCKER_REGISTRY) \
   	--build-arg BUILD_GIT_VERSION=$$(git describe --tags --always --dirty) \
   	--build-arg BUILD_GIT_COMMIT=$$(git rev-parse --short HEAD) \
   	--build-arg BUILD_DATE=$(BUILD_DATE) \
   	--build-arg SOURCE_DATE_EPOCH=$(SOURCE_DATE_EPOCH) \
   	-t $(DOCKER_REGISTRY)/$(IMAGE):$(VERSION) \
   	-f Dockerfile . \
   	--output type=docker,rewrite-timestamp=true,unpack=false
   else
   	BUILD_TARGET='$(BUILD_TARGET)' \
   	NAMESPACE='$(BUILD_NAMESPACE)' \
   	TIMEOUT='$(BUILD_TIMEOUT)' \
   	REPO_URL='$(BUILD_REPO_URL)' \
   	REF='$(REF)' \
   	TAG='$(VERSION)' \
   	DOCKERFILE='$(BUILD_DOCKERFILE)' \
   	IMAGE='$(DOCKER_REGISTRY)/$(IMAGE)' \
   	PUSH='true' \
   	PUSH_SECRET='$(BUILD_PUSH_SECRET)' \
   	hack/build-cluster.sh
   endif
   ```
   Recipe lines are TAB-indented; the `ifeq`, `else` and `endif` lines start in column 1 with no leading whitespace. Preserve these properties exactly, because the spec's ACs anchor on them:
   - `build:` keeps both prerequisites `check-version-tag check-go-mod`, on one line, at column 1.
   - `--pull` does NOT appear on any flag line.
   - `--rm=true` does NOT appear at all.
   - `--no-cache` appears exactly once, on its own flag line.
   - `--platform=linux/amd64` is retained.
   - All five `--build-arg` flags are present, one per line: `DOCKER_REGISTRY`, `BUILD_GIT_VERSION`, `BUILD_GIT_COMMIT`, `BUILD_DATE`, `SOURCE_DATE_EPOCH`.
   - `docker buildx build` appears exactly once.
   - `hack/build-cluster.sh` appears exactly once.
   - The `TAG=` line is exactly `TAG='$(VERSION)'` — the repository's own tag variable, not the reference's `SERVICE`/`BRANCH` naming.
   - `--output type=docker,rewrite-timestamp=true,unpack=false` follows the `.` context argument, mirroring the reference.

4. **Keep the cluster arm's environment mapping to the script's own variable names.** The script reads `NAMESPACE`, `TIMEOUT`, `REPO_URL`, `REF`, `TAG`, `DOCKERFILE`, `IMAGE`, `PUSH`, `PUSH_SECRET` — not `BUILD_*` — so the `BUILD_*` variables above are mapped onto them in the recipe exactly as written in requirement 3. Do NOT rename a script variable, do NOT add a variable the script does not read, and do NOT export them with `export` — the inline `NAME='value' \` prefix form is what the reference uses.

   Two value shapes are load-bearing and look wrong if you do not read the script, so keep them exactly as requirement 3 writes them:
   - `IMAGE` is the **full repository including the registry** — `'$(DOCKER_REGISTRY)/$(IMAGE)'` — because `hack/build-job.yaml` renders `name=__IMAGE__:__TAG__` from it. Passing the bare `'$(IMAGE)'` would push to `bborbe/agent-claude` instead of `docker.io/bborbe/agent-claude`.
   - `PUSH` is the **boolean** `'true'`. `hack/build-cluster.sh` validates it with `case "${PUSH}" in true|false) ;; *) die "PUSH '…' must be true or false"` and the manifest renders `push=__PUSH__`. Passing the image name there aborts the build before the Job is created.

5. **Leave `check-version-tag`, `check-go-mod`, `upload`, `clean`, `apply` and `buca` byte-for-byte unchanged.** In particular:
   - `check-go-mod` still runs `go mod vendor` (the spec retains it: it keeps the local arm's vendored build and `clean`'s `rm -rf vendor` unchanged).
   - `check-version-tag` still refuses a version that is not `HEAD`'s tag unless `ALLOW_UNTAGGED_BUILD` is set.
   - `upload` still runs `docker push` unconditionally and `clean` still runs its four local commands. Prompt 3 of this spec guards them; do not pre-empt it.
   - `buca: build upload clean apply` is unchanged.

6. **Do not add a `BUILD_TARGET` guard to any target other than `build`.** After this prompt `grep -c 'ifeq ($(BUILD_TARGET),local)' Makefile.docker` must print exactly `1`. Prompt 3 raises it to `3`.

7. **Do not add any other variable, target, `.PHONY` line or comment that names a counted string.** Specifically the file must contain zero occurrences of `SERVICE` and zero of `BRANCH` (case-sensitive), zero of `date -u`, zero of `mod=vendor`, and zero lines whose first non-whitespace characters are `--pull`. Do not add a `BUILD_CACHE`, `BUILD_PUSH`, `BUILD_PLATFORM`, `DOCKER_BUILDKIT` or `--load` flag — the spec's Non-goals rule out a persistent cache and nothing asks for a new knob.

8. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) `BUILD_TARGET ?= local` occurs once; (ii) `ifeq ($(BUILD_TARGET),local)` occurs once; (iii) the local dry run prints the buildx recipe and the cluster dry run prints `hack/build-cluster.sh`; (iv) `make -n build BUILD_TARGET=sun` without `REF` still reaches the script (the script, not make, produces the `REF is required` failure) — make must not add its own `REF` check; (v) the only file you touched is `Makefile.docker`.
</requirements>

<constraints>
- **`hack/` is a verbatim copy of `bborbe/go-skeleton` at `18866d9d`.** Do NOT edit `hack/build-cluster.sh`, `hack/build-job.yaml`, `hack/cluster-alias.sh`, `hack/build-cluster-bootstrap.sh` or `hack/dockerconfig.json.tpl`. A repo-specific need is expressed in the Makefile variables the script reads, never in the script.
- **`REF` has no default and the cluster arm must not invent one.** Do not add `REF ?= master`, do not fall back to `HEAD`, and do not compute a ref in make. `hack/build-cluster.sh` owns the `REF is required` failure, and the spec's Failure Modes table requires that exact message to reach the operator.
- **`BUILD_TARGET` defaults to `local`.** Do not default it to a cluster, do not auto-detect, and do not accept an empty value as "remote" — `ifeq ($(BUILD_TARGET),local)` must be the only local arm.
- **No `--pull`.** A moving base image would break the two-build image-ID equality this change establishes; both base images are version-pinned (`golang:1.27.3`, `alpine:3.23`) except the final stage's `FROM alpine`, which is out of scope.
- **`--no-cache` is retained.** A local build must still recompile rather than trust a stale layer. `--rm=true` is dropped: buildx removes intermediate containers unconditionally, so it is a no-op there. `--platform=linux/amd64` is retained.
- **`SOURCE_DATE_EPOCH` needs no `ARG` in the Dockerfile.** BuildKit's dockerfile frontend reads it from the build-arg set to drive the exporter's `rewrite-timestamp`, which is what the two-build image-ID equality measures. Do not add an `ARG` for it in this prompt (prompt 4 touches the Dockerfile, and only for the two documented lines).
- **The push credential is never inlined.** The cluster arm passes `PUSH_SECRET='$(BUILD_PUSH_SECRET)'` — a Secret *name*. Do not pass a value, do not read a credential, and do not add a `docker login` step.
- **`apply` deploys nothing and stays a no-op.** Do not touch it; the spec's Constraints record that `make build upload clean` and `make buca` have the same effect here.
- **No `git` command anywhere in `<verification>`.** `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails. The `$(shell git …)` calls inside `Makefile.docker` are expanded by `make -n` (they fill `SOURCE_DATE_EPOCH`, `BUILD_DATE` and `VERSION`) and resolve to empty under the masked `.git`; that is fine.
- Do NOT commit — dark-factory handles git.
- Do NOT run `docker`, `make build`, `make buca`, `kubectl`, `hack/*.sh` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling. Use `make -n` (dry run) only.
- Change ONLY `Makefile.docker`. Do NOT touch `Makefile`, `Makefile.precommit`, `Makefile.variables`, `Dockerfile`, `CHANGELOG.md`, `docs/`, `hack/`, `go.mod`, `go.sum` or any `.go` file.
- Existing tests must still pass.
- This prompt adds no Go code. If you find yourself tempted to write one, don't.
</constraints>

<verification>
Run each command from the repository root and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`.

A note on reading the results: absence checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero. The `make -n` commands render recipes without executing them, so no Docker daemon or cluster is needed; they still exit 0. Under this container's masked `.git` they also print `fatal: not a git repository (or any of the parent directories): .git` on stderr, four lines per run: two from the `$(shell git log …)` calls that fill `SOURCE_DATE_EPOCH` and `BUILD_DATE`, and two from the parse-time `git describe` / `git rev-list` in `Makefile`'s `VERSION` fallback. That is expected noise from the masked repository, not a failure: the exit code is 0, `VERSION` expands empty, and none of the assertions below depends on either value.

1. `test -f hack/build-cluster.sh && echo present` — prints `present` (the ship-order guard for prompt 1).
2. `grep -c 'BUILD_TARGET ?= local' Makefile.docker` — prints `1`.
3. `grep -c '^build: check-version-tag check-go-mod' Makefile.docker` — prints `1`.
4. `grep -c 'docker buildx build' Makefile.docker` — prints `1`.
5. `grep -c 'hack/build-cluster.sh' Makefile.docker` — prints `1`.
6. `grep -c 'rewrite-timestamp=true,unpack=false' Makefile.docker` — prints `1`.
7. `grep -c -- '--provenance=false' Makefile.docker` — prints `1`; `grep -c -- '--sbom=false' Makefile.docker` — prints `1`.
8. `grep -c -- '^[[:space:]]*--no-cache' Makefile.docker` — prints `1`; `! grep -q -- '^[[:space:]]*--pull' Makefile.docker` — exits 0 (the anchored forms, so a comment that names `--pull` neither satisfies nor breaks them — which is also why requirement 2 forbids the string in comments).
9. `! grep -q -- '--rm=true' Makefile.docker` — exits 0.
10. `! grep -q 'date -u' Makefile.docker` — exits 0; `grep -c 'SOURCE_DATE_EPOCH ?=' Makefile.docker` — prints `1`; `grep -c 'BUILD_DATE ?=' Makefile.docker` — prints `1`.
11. Each of these prints `1`: `grep -c -- '--build-arg DOCKER_REGISTRY=' Makefile.docker`, `grep -c -- '--build-arg BUILD_GIT_VERSION=' Makefile.docker`, `grep -c -- '--build-arg BUILD_GIT_COMMIT=' Makefile.docker`, `grep -c -- '--build-arg BUILD_DATE=' Makefile.docker`, `grep -c -- '--build-arg SOURCE_DATE_EPOCH=' Makefile.docker`.
12. `grep -c "TAG='\$(VERSION)'" Makefile.docker` — prints `1` (the repository's own tag variable).
13. `grep -c 'BUILD_REPO_URL ?= https://github.com/$(IMAGE).git' Makefile.docker` — prints `1` (the clone URL derives from the repository's image variable).
14. `! grep -q 'SERVICE' Makefile.docker` — exits 0; `! grep -q 'BRANCH' Makefile.docker` — exits 0 (the reference's variable names did not survive the port).
15. `grep -c 'ifeq ($(BUILD_TARGET),local)' Makefile.docker` — prints `1` (only `build` is dispatched today; prompt 3 adds the other two).
16. `! grep -q 'mod=vendor' Makefile.docker` — exits 0 (the Dockerfile's module flag is prompt 4's business, not this file's).
17. `make -n build BUILD_TARGET=local` — prints the `docker buildx build` recipe, including `--no-cache`, `--provenance=false`, `--sbom=false`, the five `--build-arg` lines and `--output type=docker,rewrite-timestamp=true,unpack=false`. It does not print `hack/build-cluster.sh`.
18. `make -n build BUILD_TARGET=sun REF=master | grep -c 'hack/build-cluster.sh'` — prints `1`; `! make -n build BUILD_TARGET=sun REF=master | grep -q 'docker buildx build'` — exits 0.
19. `make -n build BUILD_TARGET=sun | grep -c "REF=''"` — prints `1` (the empty `REF` is passed through; make adds no default and no check, so the script's own `REF is required` failure stays reachable).
20. `make -n build BUILD_TARGET=sun REF=master | grep -c "TAG='"` — prints `1` (the tag variable is mapped onto the script's `TAG`; grepping the expanded prefix keeps the check independent of whether `VERSION` resolves).
21. `make -n build BUILD_TARGET=sun REF=master | grep -c "IMAGE='docker.io/bborbe/agent-claude'"` — prints `1` (the cluster arm passes the full repository including the registry, which is what the Job renders `name=__IMAGE__:__TAG__` from).
22. `make -n build BUILD_TARGET=sun REF=master | grep -c "PUSH='true'"` — prints `1` (the script validates `PUSH` as `true`/`false`; anything else aborts before the Job is created).
23. `make -n upload BUILD_TARGET=sun | grep -c 'docker push'` — prints `1` and `make -n clean BUILD_TARGET=sun | grep -c 'docker rmi'` — prints `1` (neither target is guarded yet; prompt 3 changes that).
24. `grep -c '^buca: build upload clean apply$' Makefile.docker` — prints `1`; `grep -c 'check-version-tag check-go-mod' Makefile.docker` — prints `1`.
25. `! grep -q 'BUILD_CACHE\|--load\|DOCKER_BUILDKIT' Makefile.docker` — exits 0 (no knob the spec did not ask for).
26. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command and the exit code the completion report carries. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–7.
</verification>

<success_criteria>
- `Makefile.docker` declares `BUILD_TARGET ?= local`, `BUILD_NAMESPACE ?= build`, `BUILD_TIMEOUT ?= 3600`, `BUILD_REPO_URL ?= https://github.com/$(IMAGE).git`, `BUILD_DOCKERFILE ?= Dockerfile`, `BUILD_PUSH_SECRET ?= buildkit-push`, `REF ?=` (empty), `SOURCE_DATE_EPOCH ?= $(shell git log -1 --format=%ct)` and `BUILD_DATE ?= $(shell git log -1 --format=%cI)`, above every target.
- `build` keeps `check-version-tag check-go-mod` as prerequisites and dispatches on `BUILD_TARGET` with exactly one `ifeq ($(BUILD_TARGET),local)`.
- The local arm runs `docker buildx build --platform=linux/amd64 --no-cache --provenance=false --sbom=false` with all five `--build-arg` flags, tags `$(DOCKER_REGISTRY)/$(IMAGE):$(VERSION)`, and writes through `--output type=docker,rewrite-timestamp=true,unpack=false`; it carries no `--pull` and no `--rm=true`.
- The clock is commit-derived: `date -u` occurs zero times, `SOURCE_DATE_EPOCH ?=` and `BUILD_DATE ?=` occur once each, and `--build-arg BUILD_DATE=` / `--build-arg SOURCE_DATE_EPOCH=` are passed on the local arm.
- The cluster arm passes `BUILD_TARGET`, `NAMESPACE`, `TIMEOUT`, `REPO_URL`, `REF`, `TAG='$(VERSION)'`, `DOCKERFILE`, `IMAGE`, `PUSH` and `PUSH_SECRET` to `hack/build-cluster.sh`, and invents no `REF` default.
- `SERVICE` and `BRANCH` occur zero times in `Makefile.docker`; `upload`, `clean`, `apply`, `buca`, `check-version-tag` and `check-go-mod` are byte-for-byte unchanged.
- `make -n build BUILD_TARGET=local` prints the buildx recipe and `make -n build BUILD_TARGET=sun REF=master` prints the `hack/build-cluster.sh` invocation, each without executing anything.
- Only `Makefile.docker` is modified; `ROOTDIR=/workspace make precommit` exits 0 and every command in `<verification>` prints the stated result.
</success_criteria>

<completion_report>
End your run with the dark-factory completion report block:
- `"status": "success"` only if `ROOTDIR=/workspace make precommit` exited 0 and every verification command printed the stated result.
- `"status": "failed"` if any requirement is unmet (including requirement 1's missing-`hack/` precondition) or if any verification command did not print the stated result — including a non-zero `make precommit`, with no exceptions and no "pre-existing failure" rationalisation.
- Include `"verification": {"command": "ROOTDIR=/workspace make precommit", "exitCode": <N>}` with the actual exit code.

Then append an `## Improvements` section, one bullet per observation, each tagged `[PROMPT]`, `[GUIDE]` or `[GLOBAL]`. If there is genuinely nothing to suggest, write `- None`.
</completion_report>
