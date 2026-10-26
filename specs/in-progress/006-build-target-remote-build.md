---
status: verifying
approved: "2026-10-26T01:34:55Z"
generating: "2026-10-26T01:34:55Z"
prompted: "2026-10-26T01:47:51Z"
verifying: "2026-10-26T02:22:54Z"
branch: dark-factory/build-target-remote-build
---

## Summary

- Building the image gains a target switch: by default it still happens on the invoking machine, and any other value builds it inside a named cluster.
- The cluster path runs the build as a Kubernetes Job that holds the registry credential inside the pod, so the credential never sits on the invoking machine.
- The local path becomes reproducible in what the recipe controls: two builds of one commit produce an identical binary and an identical image config, which plain `docker build` cannot do. The image ID itself is not compared — an image that warms a toolchain cache ships files whose contents embed build timestamps, and no exporter flag rewrites those.
- The push and cleanup steps become no-ops on a remote target, because the Job already pushed and no local image exists.
- The image stops requiring a vendored tree, and stops defaulting its base-image registry to a machine-specific host.

## Problem

`bborbe/agent-claude` is the only machine-bound build left among the repos whose build the fleet is moving off the Mac. Its `Makefile.docker` runs `DOCKER_BUILDKIT=1 docker build --pull --no-cache` on the invoking machine, so cutting a release image requires that machine's Docker daemon. `bborbe/go-skeleton` already carries the target-side implementation — a parameterised BuildKit Job, a bootstrap script, and a buildx local arm — and its `docs/build-migration-guide.md` describes the port. What is missing is the port itself, done for a repo whose variables, prerequisites and build context differ from the reference in ways the guide predicts but does not resolve.

## Goal

`make build BUILD_TARGET=local` and `make build BUILD_TARGET=<cluster>` both produce an image from the same ref — the same commit when `REF` names `HEAD` — and the local path keeps working exactly as it does today. On the cluster arm the build runs as a Job in the target cluster, pushes `docker.io/bborbe/agent-claude` under the account the pod holds, and leaves no image on the invoking machine.

Cross-arm digest equality is **not** a goal. The reference Job passes only the commit epoch as a build arg, so a cluster-built image carries the Dockerfile's `dev` / `none` / `unknown` version defaults while the local arm stamps real values; the two arms are equivalent in where they build, not in the bytes they emit. That divergence is recorded as a gap, not closed here.

## Non-goals

- **Not a change to what the image contains.** No new package, no version bump, no Dockerfile stage change beyond the module-resolution flag and the base-registry default.
- **Not a deployment change.** `apply` deploys nothing, and the cluster still pulls the mirrored quant-registry image rather than docker.io.
- **Not a change to `bborbe/go-skeleton`.** The reference implementation is copied, not forked, and `go-skeleton` is not edited.
- **Not a release.** Verification runs on a scratch `VERSION`; the release is cut by the maintainer after merge.
- **Not a persistent build cache.** Every remote build pays the cold-cache cost; that is the reference design and is not changed here.
- **Not the version-label gap.** Making a cluster build stamp `BUILD_GIT_VERSION` / `BUILD_GIT_COMMIT` / `BUILD_DATE` requires the reference `hack/build-job.yaml` to pass three more build args. Editing it would fork the reference, so the gap is recorded for the fleet task instead.
- **Not base-image pinning.** The final stage's unqualified `FROM alpine` is a pre-existing reproducibility defect. It is recorded, not fixed here.
- **Not `make generatek8s`.** It invokes `hack/update-codegen.sh`, which does not exist and is not among the five files this change vendors. That breakage predates this change and stays.

## Acceptance Criteria

- [ ] The five cluster-build scripts are byte-identical copies of the pinned reference, and no other file sits under `hack/` — evidence: `ls hack/ | wc -l` returns 5, and this loop exits 0:
  ```
  for f in build-cluster.sh build-cluster-bootstrap.sh build-job.yaml cluster-alias.sh dockerconfig.json.tpl; do
    curl -sSfL "https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack/$f" | diff - "hack/$f" || exit 1
  done
  ```
  A stub file, or any local edit to a vendored script, fails this diff.
- [ ] The three shell scripts carry their reference mode — evidence: `stat -c '%a %n' hack/build-cluster-bootstrap.sh hack/build-cluster.sh hack/cluster-alias.sh` prints `755` for each, and `stat -c '%a %n' hack/build-job.yaml hack/dockerconfig.json.tpl` prints `644` for each.
- [ ] `build` dispatches on `BUILD_TARGET` and keeps its prerequisites — evidence: `grep -c 'BUILD_TARGET ?= local' Makefile.docker` returns 1, `grep -c 'hack/build-cluster.sh' Makefile.docker` returns 1, `grep -c 'docker buildx build' Makefile.docker` returns 1, and `grep -c '^build: check-version-tag check-go-mod' Makefile.docker` returns 1.
- [ ] The local arm uses the reproducible exporter recipe and drops `--pull` — evidence: `grep -c 'rewrite-timestamp=true,unpack=false' Makefile.docker` returns 1, `grep -c -- '--provenance=false' Makefile.docker` returns 1, `grep -c -- '--sbom=false' Makefile.docker` returns 1, `grep -c -- '^[[:space:]]*--no-cache' Makefile.docker` returns 1, and `grep -c -- '^[[:space:]]*--pull' Makefile.docker` returns 0 — the last two anchored to flag lines so a comment that names `--pull` neither satisfies nor breaks them.
- [ ] The build clock comes from the commit, not the wall clock — evidence: `grep -c 'date -u' Makefile.docker` returns 0, `grep -c 'SOURCE_DATE_EPOCH ?=' Makefile.docker` returns 1, and `grep -c 'BUILD_DATE ?=' Makefile.docker` returns 1.
- [ ] All five build args are passed on the local arm — evidence: `grep -c -- '--build-arg DOCKER_REGISTRY=' Makefile.docker`, `grep -c -- '--build-arg BUILD_GIT_VERSION=' Makefile.docker`, `grep -c -- '--build-arg BUILD_GIT_COMMIT=' Makefile.docker`, `grep -c -- '--build-arg BUILD_DATE=' Makefile.docker` and `grep -c -- '--build-arg SOURCE_DATE_EPOCH=' Makefile.docker` each return 1.
- [ ] The repo's own variable names are preserved, not the reference's — evidence: `grep -c "TAG='\$(VERSION)'" Makefile.docker` returns 1, `grep -c 'BUILD_REPO_URL ?= https://github.com/\$(IMAGE).git' Makefile.docker` returns 1, and `grep -c 'SERVICE' Makefile.docker` and `grep -c 'BRANCH' Makefile.docker` each return 0.
- [ ] `upload` and `clean` are guarded for a remote target — evidence: `grep -c 'ifeq (\$(BUILD_TARGET),local)' Makefile.docker` returns 3, one for each of `build`, `upload` and `clean`.
- [ ] The Dockerfile needs no vendored tree and no machine-specific base registry — evidence: `grep -c 'mod=vendor' Dockerfile` returns 0 and `grep -c '^ARG DOCKER_REGISTRY=docker.io$' Dockerfile` returns 1.
- [ ] The two docs that describe the build are brought up to date — evidence: `grep -c 'docker.prod.nuke.benjamin-borbe.de' docs/local-docker-run.md` returns 0, `grep -c 'go mod vendor' docs/local-docker-run.md` returns 0, and in `docs/publish-image.md` both `grep -c 'BUILD_TARGET'` and `grep -c 'REF='` return ≥ 1 and `grep -c 'build-cluster-bootstrap.sh'` returns ≥ 1.
- [ ] The change is recorded — evidence: `grep -c '^## Unreleased' CHANGELOG.md` returns 1 and `awk '/^## Unreleased/{f=1} f && /BUILD_TARGET/{print; exit}' CHANGELOG.md` prints a line.
- [ ] `make precommit` exits 0 — evidence: exit code.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

The container has no Docker daemon and its `.git` is masked, so nothing here runs a build or reads git. It carries `go`, `make`, `bash`, `trivy`, `jq`, `python3`, `curl`, `stat`, `grep`, `awk` and `diff`; `column` is absent, but only the `vulncheck` failure path uses it.

- `make precommit` — `ensure`, `format`, `generate`, `python-check`, `python-test`, `shell-test`, `test`, `check` and `addlicense` all clean
- `bash -n` on the three vendored shell scripts — they parse
- `stat -c '%a %n' hack/*` — the five files at their reference modes
- the `curl | diff` loop in AC 1 — byte-identity with the pinned reference
- the `grep` assertions over `Makefile.docker`, `Dockerfile`, `docs/` and `CHANGELOG.md` named in the ACs above

Note that CI never builds the image — `.github/workflows/ci.yml` runs only `make precommit` — so the greps are the whole automated evidence, and the operator rung below is the only place the image is actually built.

### Operator-executable (runs on the host, before the PR)

Prerequisites, all of which the reference guide lists: `kubectl` plus a kubeconfig for the target cluster, `crane`, `teamvault-cli` with a login for the account holding the registry key, and `docker login` as `bborbe` for the local arm's push.

- `for f in build-cluster.sh build-cluster-bootstrap.sh build-job.yaml cluster-alias.sh dockerconfig.json.tpl; do diff <(git show HEAD:hack/$f) <(git -C "${GO_SKELETON:?set GO_SKELETON to a local go-skeleton checkout}" show 18866d9d6b0d6159829f7a9e825232608067fee7:hack/$f) || echo "DIFF $f"; done` — prints nothing. `GO_SKELETON` names a local `bborbe/go-skeleton` checkout and has no default, so the rung fails loudly rather than resolving against the wrong repository; where no checkout exists, the container-side equivalent is the pinned raw-URL fetch in Constraints, and AC 1's `curl | diff` is the same proof. **Do not hard-code a home-directory path here** — this rung has to run on whatever machine is doing the migration.
- `hack/build-cluster-bootstrap.sh <cluster>` — exits 0, and `KUBECONFIG=~/.kube/<cluster> kubectl --context <ctx> -n build get secret -o name` lists `buildkit-push` (names only, never values)
- `ALLOW_UNTAGGED_BUILD=1 VERSION=proof make build BUILD_TARGET=local` twice, comparing the two images rather than their IDs: `docker run --rm <id> sha256sum /main` prints the same digest both times, and `docker image inspect -f '{{json .Config}}' <id>` prints the same config. The image IDs themselves will differ — see the Constraint on the warm build cache.
- `ALLOW_UNTAGGED_BUILD=1 VERSION=proof REF=<branch> make build BUILD_TARGET=<cluster>` — exits 0 and prints a digest; `crane digest docker.io/bborbe/agent-claude:proof` returns that digest
- `kubectl -n build get pod -l job-name=build-proof -o jsonpath='{.items[0].spec.nodeName}'` — names a node in the target cluster, not the Mac. This is the check a local build plus a local push cannot fake.
- `ALLOW_UNTAGGED_BUILD=1 VERSION=proof make buca BUILD_TARGET=local` and `ALLOW_UNTAGGED_BUILD=1 VERSION=proof REF=<branch> make buca BUILD_TARGET=<cluster>` — both exit 0. The goal's Definition of Done names `make buca` itself, so it is run once on a scratch tag: the repo's docs ban it for *release* publishing (`docs/publish-image.md:36`, `docs/local-docker-run.md:12`), and since `apply` deploys nothing the only side effect is the push, which the scratch tag confines and the next step removes.
- `crane delete docker.io/bborbe/agent-claude:proof` — the scratch tag is gone from the public repository. `clean` cannot do this; it removes only the local image.

## Desired Behavior

1. **One variable selects the machine.** `BUILD_TARGET` defaults to `local`; any other value is resolved by `hack/cluster-alias.sh` to a kubeconfig and context — `sun`, `nukedev`, `nukeprod`, or a raw context name — and the build runs as a Job there.
2. **The cluster arm renders a per-build Job.** `hack/build-cluster.sh` substitutes the repo URL, ref, tag, image, Dockerfile path, push secret, deadline and commit epoch into `hack/build-job.yaml`, refuses to apply when a Job of that name already exists, waits for it to succeed or fail, and relays the digest lines from the build log. `REF` is a required input on this arm and has no default.
3. **The local arm is reproducible in what the recipe controls.** `docker buildx build` writes through the `type=docker` exporter with `rewrite-timestamp=true,unpack=false`, with provenance and SBOM attestations disabled, so two builds of one commit yield the same binary and the same image config. `BUILD_DATE` is the commit's date, not the wall clock. The image ID is not part of the claim: `rewrite-timestamp` rewrites the layer tar's timestamps, not timestamps inside file contents, so any image that ships a warmed toolchain cache is not byte-reproducible.
4. **A remote build pushes and never loads.** The image exists in the registry and nowhere on the invoking machine, so `upload` and `clean` skip their local work when `BUILD_TARGET` is not `local`.
5. **The repo's own variables survive the port.** The image name comes from `IMAGE`, the tag from `VERSION`, and the clone URL from `https://github.com/$(IMAGE).git`; the **local arm** of `build` keeps `check-version-tag` and `check-go-mod` as prerequisites. They are local-arm only because both describe the local HEAD — `check-version-tag` asserts the tree sits at the tag it claims to build, and `check-go-mod` runs `go mod vendor` — while the cluster arm builds `REF`, which need not be HEAD and need not exist locally at all. Applying them to the cluster arm would validate a different commit than the one built, and would demand a local Go toolchain and network for a build that happens in the cluster.
6. **The image builds without a vendored tree, off a portable base registry.** The `go build` in the Dockerfile resolves modules from the module cache when `vendor/` is absent — the state of every cluster clone — and uses `vendor/` when it is present; the base-image default is `docker.io` rather than a machine-specific mirror host.

## Constraints

- **`hack/` is a verbatim copy of `bborbe/go-skeleton` at `18866d9d6b0d6159829f7a9e825232608067fee7`** — `build-cluster.sh`, `build-cluster-bootstrap.sh`, `build-job.yaml`, `cluster-alias.sh`, `dockerconfig.json.tpl`. No local edit. A repo-specific need is expressed in the Makefile variables the script reads, never in the script; that is what keeps this spec reusable for the next repo.
- **The container cannot read the go-skeleton checkout.** The dark-factory container mounts only this repo, runs with `.git` masked and has no Docker daemon, so the five files are fetched from `https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack/<file>` and the three shell scripts made executable. The pinned commit is what makes the copy reproducible, and AC 1's `curl | diff` is what proves it is byte-identical.
- **The reference's `BUILD_*` variables are declared in full — except the namespace** — `BUILD_TARGET`, `BUILD_TIMEOUT`, `BUILD_REPO_URL`, `BUILD_DOCKERFILE`, `BUILD_PUSH_SECRET`, `REF`, `SOURCE_DATE_EPOCH`, `BUILD_DATE` — for parity with the reference, so this spec renders against the next repo unchanged. `build-cluster.sh` supplies defaults for four of them; declaring them here is what lets an operator override one without editing the script. **`BUILD_NAMESPACE` is deliberately absent**: the vendored `hack/build-job.yaml` hardcodes `namespace: build` while `build-cluster.sh` polls whatever namespace it is handed, so an override creates the Job in `build` and polls elsewhere until the 3600 s deadline expires — a silent stall that reads as a hung build. Until the manifest is templated upstream, the namespace is fixed and the script's own default applies.
- **The vendored cluster scripts carry two known upstream defects.** Both were found by review and confirmed by reading the files; neither is fixed here, because `hack/` is a byte-identical copy and patching it locally would break AC 1 *and* turn vendoring into "every repo patches its own copy" — the exact outcome the verbatim pin exists to prevent. They belong in `bborbe/go-skeleton`, and every repo that adopts this spec inherits them:
  - `hack/build-cluster-bootstrap.sh` renders the dockerconfig JSON with `printf` and no escaping. A TeamVault credential containing a quote, backslash or control character produces invalid JSON, or injects extra `auths` fields. The temp-file handling around it is sound (mode 600, unset after use, removed on every exit path).
  - No automated test covers that script's template-key extraction or JSON rendering, and `Makefile.precommit` globs only `agent/scripts/*_test.sh`, so no `hack/` script is executed by CI at all — a malformed template or credential renders a wrong secret silently. Extending that glob to `hack/*_test.sh` is the fix, and it is additive rather than an edit to the vendored files, so it is available to any adopter without breaking the pin.
- **The push credential is named, never inlined.** `hack/dockerconfig.json.tpl` names a TeamVault lookup key; `hack/build-cluster-bootstrap.sh` resolves it with `teamvault-cli` into a `600` temp file, uses it for one `kubectl create secret --dry-run`, and removes it on every exit path.
- **`apply` deploys nothing.** `DIRS` resolves to `cmd`; `cmd/Makefile` includes `Makefile.folder`, whose `DIRS` resolves to `run-task`; and `cmd/run-task/Makefile`'s `apply` target is an explicit echo-only no-op. So `apply` recurses one level and deploys nothing, which is why `make build upload clean` and `make buca` have the same effect here.
- **The local arm keeps its current cleaning behaviour** — `docker image prune -f`, `docker builder prune --all -f`, `rm -rf vendor` — so a local build cleans what it cleans today.
- **`check-go-mod` is retained even though the Dockerfile no longer requires `vendor/`.** It keeps the local arm's vendored build and `clean`'s `rm -rf vendor` unchanged, and this change does not reorganise module handling.
- **No `--pull`.** A moving base image would break the binary-and-config equality this change establishes, and both base images are version-pinned (`golang:1.27.3`, `alpine:3.23`) except the final stage's `FROM alpine`, which is out of scope.
- **`--no-cache` is retained**, so a local build still recompiles rather than trusting a stale layer. `--rm=true` is dropped: buildx removes intermediate containers unconditionally, so it is a no-op there. `--platform=linux/amd64` is retained.
- **`SOURCE_DATE_EPOCH` needs no `ARG` in the Dockerfile.** BuildKit's dockerfile frontend reads it from the build-arg set to drive the exporter's `rewrite-timestamp`, which normalises the layer tar's timestamps — not the timestamps inside the files that tar carries.
- **A warmed toolchain cache bounds reproducibility, and the bound is not the recipe's to close.** `rewrite-timestamp` rewrites timestamps in the layer tar; it cannot rewrite timestamps inside file contents. This repo's image deliberately ships a warm Go build cache (`Dockerfile:112-120` — a pod's first `make check` is 70 s warm versus 601 s cold), and cache entries embed build timestamps, so two builds of one commit differ in **16,216 files, all under `/home/claude/.cache/go-build`**, while the compiled binary and the image config are identical. Measured on this repo 2026-10-26. The consequence for this spec's reusability: **the reproducible claim covers the binary and the image config, never the image ID**, and the next repo inherits the same limit the moment its image warms a cache. Do not attempt to close it by deleting the cache — that trades a reproducibility assertion nobody consumes for 531 s on every pod's first check.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `REF` is unset on the cluster arm | `hack/build-cluster.sh` exits non-zero before rendering: `REF is required; pass it on the make command line` | Pass `REF=<branch or tag>`; a release build must name the tag |
| `BUILD_TARGET` names a context the resolved kubeconfig does not know | `hack/build-cluster.sh` exits non-zero before rendering, naming the target and listing the accepted values | Correct the value; `sun`/`nukedev`/`nukeprod` are aliases, anything else is a raw context name |
| `BUILD_TARGET` resolves to a reachable cluster that is not the intended one | The build succeeds and pushes into the wrong cluster; the exit code is green and nothing warns | Confirm `kubectl -n build get pod -l job-name=build-<tag> -o jsonpath='{.items[0].spec.nodeName}'` names a node of the intended cluster |
| The `build` namespace or push secret is missing | The Job's pod stays `Pending` with a `FailedMount` event and the outer `make` reports the Job as not succeeding | Run `hack/build-cluster-bootstrap.sh <target>`; it is idempotent |
| The `buildkit-push` secret is replaced with the reflector's `docker` secret | The push fails with a registry permission error that reads like a build bug; that secret is a mirror of a different registry | Re-provision with `hack/build-cluster-bootstrap.sh <target>`, which writes `buildkit-push` |
| A Job with the derived name already exists | `hack/build-cluster.sh` refuses and names the delete command | `kubectl -n build delete job <name>`, then re-run |
| `REF` resolves to a different commit in the cluster than locally | The build container compares the cloned epoch against the rendered one and exits 1 naming both values | `git fetch`, then re-run |
| The init container's clone trips git's ownership guard | The Job reports `Running` while the build never starts; the init container exits 128 with `detected dubious ownership` | The vendored `build-job.yaml` already carries `GIT_CONFIG_*` for `/src`; a hand-rolled manifest would not |
| The active buildx builder is not a `docker`-driver builder | `docker buildx build` writes to a separate cache, so the config equality does not hold (the compiled binary is unaffected — same source, Dockerfile and build args) | `docker buildx ls` and switch to the `*`-marked docker-driver builder |
| docker.io rate-limits or is unavailable during a build | Both arms fail at a base-image pull; the cluster arm fails inside the Job and the local arm fails before the build starts | Wait out the window, or authenticate the pull; the base images are version-pinned, so a retry is safe |
| `VERSION` names a tag that is not `HEAD` and `ALLOW_UNTAGGED_BUILD` is unset | `check-version-tag` exits 1 naming both the HEAD tag and the requested version | Build from the tag, or set `ALLOW_UNTAGGED_BUILD=1` for a scratch build |
| `vendor/` is absent in the cluster clone | The Dockerfile's `go build` downloads modules instead of failing | None needed; this is the behaviour the change establishes |
| The pinned raw URL is unreachable from the container | The fetch step fails and no `hack/` file is written | Operator copies the five files from the local `go-skeleton` checkout at `18866d9d` into `hack/`, then re-runs the prompt |

## Security / Abuse Cases

- The push credential exists only inside the cluster. `hack/dockerconfig.json.tpl` names a TeamVault key, the bootstrap script renders it into a `600` temp file for one dry-run and deletes it on every exit path, and the Job mounts it read-only at `/docker-config`. Nothing in the repo, the logs or the Makefile carries the value.
- The cluster clones the source repo anonymously. `bborbe/agent-claude` is public, which is why no git credential is needed inside the pod.
- The five reference scripts are fetched over HTTPS from a public repository at a pinned commit, so the fetch is reproducible and cannot be redirected by a branch moving.
- The scratch tag published during verification is public and must be deleted afterwards; `clean` removes only the local image.

## Suggested Decomposition

Prompts should be generated in this order — each row is a single prompt with a clear scope.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Vendor the five cluster-build scripts under `hack/` at their reference modes | 1, 2 | 1, 2 | — |
| 2 | Dispatch `build` on `BUILD_TARGET`: the buildx local arm, the cluster arm, and the repo's variable names and prerequisites | 1, 3, 5 | 3, 4, 5, 6, 7 | prompt 1 |
| 3 | Guard `upload` and `clean` for a remote target | 4 | 8 | prompt 2 |
| 4 | Build the image without a vendored tree, off a portable base registry | 6 | 9 | — |
| 5 | Document the build targets and record the change | 3 | 10, 11 | prompts 2, 3, 4 |

Rationale: prompt 1 lands the scripts the cluster arm calls, so prompt 2 can reference them without also authoring them. Prompt 3 depends on prompt 2's `BUILD_TARGET` block existing in the same file. Prompt 4 is independent. Prompt 5 records what the first four produced.

## Do-Nothing Option

`make build` keeps requiring the invoking machine's Docker daemon, and every release image of the worker that runs the fleet's agents stays tied to one Mac. The reference implementation exists and is proven on `go-skeleton`, so the cost of not doing this is not risk — it is that the fleet rollout has no worked example of the port on a repo whose variables and build context differ from the reference, and the next repo pays the same discovery cost this one would have paid once.
