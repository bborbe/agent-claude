---
status: draft
---

## Summary

- `make build` gains a `BUILD_TARGET` switch: `local` (the default) keeps building on the invoking machine, any other value builds the same commit inside a cluster.
- The cluster arm runs a `buildctl-daemonless.sh` Job that mounts the docker.io push credential inside the pod, so the credential never sits on the invoking machine.
- The local arm moves from plain `docker build` to `docker buildx build` with a timestamp-rewriting exporter, so two builds of one commit produce the same image ID.
- `upload` and `clean` become no-ops on a remote target, because the Job already pushed and no local image exists.
- The Dockerfile stops requiring a vendored tree and stops defaulting its base-image registry to a machine-specific host.

## Problem

`bborbe/agent-claude` is the only machine-bound build left among the repos whose build the fleet is moving off the Mac. Its `Makefile.docker` runs `DOCKER_BUILDKIT=1 docker build --pull --no-cache` on the invoking machine, so cutting a release image requires that machine's Docker daemon. `bborbe/go-skeleton` already carries the target-side implementation — a parameterised BuildKit Job, a bootstrap script, and a buildx local arm — and its `docs/build-migration-guide.md` describes the port. What is missing is the port itself, done for a repo whose variables, prerequisites and build context differ from the reference in ways the guide predicts but does not resolve.

## Goal

`make build BUILD_TARGET=local` and `make build BUILD_TARGET=<cluster>` both produce an image from the same commit, and the local path keeps working exactly as it does today. On the cluster arm the build runs as a Job in the target cluster, pushes `docker.io/bborbe/agent-claude` under the account the pod holds, and leaves no image on the invoking machine.

Cross-arm digest equality is **not** a goal. The reference Job passes only the commit epoch as a build arg, so a cluster-built image carries the Dockerfile's `dev` / `none` / `unknown` version defaults while the local arm stamps real values; the two arms are equivalent in where they build, not in the bytes they emit. That divergence is recorded as a gap, not closed here.

## Non-goals

- **Not a change to what the image contains.** No new package, no version bump, no Dockerfile stage change beyond the module-resolution flag and the base-registry default.
- **Not a deployment change.** `apply` stays a no-op, and the cluster still pulls the mirrored quant-registry image rather than docker.io.
- **Not a change to `bborbe/go-skeleton`.** The reference implementation is copied, not forked, and `go-skeleton` is not edited.
- **Not a release.** Verification runs on a scratch `VERSION`; the release is cut by the maintainer after merge.
- **Not a persistent build cache.** Every remote build pays the cold-cache cost; that is the reference design and is not changed here.
- **Not the version-label gap.** Making a cluster build stamp `BUILD_GIT_VERSION` / `BUILD_GIT_COMMIT` / `BUILD_DATE` requires the reference `hack/build-job.yaml` to pass three more build args. Editing it would fork the reference, so the gap is recorded for the fleet task instead.
- **Not base-image pinning.** The final stage's unqualified `FROM alpine` is a pre-existing reproducibility defect. It is recorded, not fixed here.

## Acceptance Criteria

- [ ] The five cluster-build scripts exist at their reference paths, with their reference modes, and carry the reference's behaviour — evidence: `ls hack/ | wc -l` returns 5; `stat -c '%a %n' hack/build-cluster-bootstrap.sh hack/build-cluster.sh hack/cluster-alias.sh` prints `755` for each and `stat -c '%a %n' hack/build-job.yaml hack/dockerconfig.json.tpl` prints `644` for each; `grep -c 'buildctl-daemonless.sh build' hack/build-job.yaml` returns 1; `grep -c 'safe.directory' hack/build-job.yaml` returns 1; `grep -c 'delete job' hack/build-cluster.sh` returns 1; `grep -c 'teamvault-cli' hack/build-cluster-bootstrap.sh` returns ≥ 1.
- [ ] Each shell script parses — evidence: `bash -n hack/build-cluster.sh && bash -n hack/build-cluster-bootstrap.sh && bash -n hack/cluster-alias.sh` exits 0.
- [ ] `build` dispatches on `BUILD_TARGET` and keeps its prerequisites — evidence: `grep -c 'BUILD_TARGET ?= local' Makefile.docker` returns 1, `grep -c 'hack/build-cluster.sh' Makefile.docker` returns 1, `grep -c 'docker buildx build' Makefile.docker` returns 1, and `grep -c '^build: check-version-tag check-go-mod' Makefile.docker` returns 1.
- [ ] The local arm uses the reproducible exporter recipe and drops `--pull` — evidence: `grep -c 'rewrite-timestamp=true,unpack=false' Makefile.docker` returns 1, `grep -c -- '--provenance=false' Makefile.docker` returns 1, `grep -c -- '--sbom=false' Makefile.docker` returns 1, `grep -c -- '^[[:space:]]*--no-cache' Makefile.docker` returns 1, and `grep -c -- '^[[:space:]]*--pull' Makefile.docker` returns 0 — the last two anchored to flag lines so a comment that names `--pull` cannot satisfy or break them.
- [ ] The build clock comes from the commit, not the wall clock — evidence: `grep -c 'date -u' Makefile.docker` returns 0, `grep -c 'SOURCE_DATE_EPOCH ?=' Makefile.docker` returns 1, and `grep -c 'BUILD_DATE ?=' Makefile.docker` returns 1.
- [ ] All five build args are passed on the local arm — evidence: `grep -c -- '--build-arg DOCKER_REGISTRY=' Makefile.docker`, `grep -c -- '--build-arg BUILD_GIT_VERSION=' Makefile.docker`, `grep -c -- '--build-arg BUILD_GIT_COMMIT=' Makefile.docker`, `grep -c -- '--build-arg BUILD_DATE=' Makefile.docker` and `grep -c -- '--build-arg SOURCE_DATE_EPOCH=' Makefile.docker` each return 1.
- [ ] The repo's own variable names are preserved, not the reference's — evidence: `grep -c "TAG='\$(VERSION)'" Makefile.docker` returns 1, `grep -c 'BUILD_REPO_URL ?= https://github.com/\$(IMAGE).git' Makefile.docker` returns 1, and `grep -c 'SERVICE' Makefile.docker` and `grep -c 'BRANCH' Makefile.docker` each return 0.
- [ ] `upload` and `clean` are guarded for a remote target — evidence: `grep -c 'ifeq (\$(BUILD_TARGET),local)' Makefile.docker` returns 3, one for each of `build`, `upload` and `clean`.
- [ ] The Dockerfile needs no vendored tree and no machine-specific base registry — evidence: `grep -c 'mod=vendor' Dockerfile` returns 0 and `grep -c '^ARG DOCKER_REGISTRY=docker.io$' Dockerfile` returns 1.
- [ ] The change is documented and recorded — evidence: `grep -c 'BUILD_TARGET' docs/publish-image.md` returns ≥ 1, `grep -c 'mod=vendor' docs/local-docker-run.md` returns 0, and `grep -c '^## Unreleased' CHANGELOG.md` returns 1 with `awk '/^## Unreleased/{f=1} f && /BUILD_TARGET/{print; exit}' CHANGELOG.md` printing a line.
- [ ] `make precommit` exits 0 — evidence: exit code.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

The container has no Docker daemon and its `.git` is masked, so nothing here runs a build or reads git.

- `make precommit` — format, generate, python-check, python-test, shell-test, test, check, addlicense all clean. The container carries `go`, `make`, `trivy`, `jq` and `python3`; `column` is absent, but only the `vulncheck` failure path uses it.
- `bash -n hack/build-cluster.sh && bash -n hack/build-cluster-bootstrap.sh && bash -n hack/cluster-alias.sh` — the scripts parse
- `stat -c '%a %n' hack/*` — the five files at their reference modes
- the `grep` assertions over `hack/*`, `Makefile.docker`, `Dockerfile`, `docs/` and `CHANGELOG.md` named in the ACs above

### Operator-executable (runs on the host after PR merge, spec verification ladder)

`make buca` is **banned** by this repo's own docs (`docs/publish-image.md:36`, `docs/local-docker-run.md:12`). Its sanctioned equivalent here is `make build upload clean`: `apply` is a no-op either way, so the chain does exactly what `buca` would.

- `for f in build-cluster.sh build-cluster-bootstrap.sh build-job.yaml cluster-alias.sh dockerconfig.json.tpl; do diff <(git show HEAD:hack/$f) <(git -C ~/Documents/workspaces/go-skeleton show 18866d9d6b0d6159829f7a9e825232608067fee7:hack/$f) || echo "DIFF $f"; done` — prints nothing: byte-identical copies of the reference, not a fork
- `hack/build-cluster-bootstrap.sh <cluster>` — exits 0, and `KUBECONFIG=~/.kube/<cluster> kubectl --context <ctx> -n build get secret -o name` lists `buildkit-push` (names only, never values)
- `ALLOW_UNTAGGED_BUILD=1 VERSION=proof make build BUILD_TARGET=local` twice, reading `docker images --no-trunc -q docker.io/bborbe/agent-claude:proof` after each — the two image IDs are equal
- `ALLOW_UNTAGGED_BUILD=1 VERSION=proof make build upload clean BUILD_TARGET=local` — exits 0
- `ALLOW_UNTAGGED_BUILD=1 VERSION=proof REF=<branch> make build BUILD_TARGET=<cluster>` — exits 0 and prints a digest; `crane digest docker.io/bborbe/agent-claude:proof` returns that digest
- `kubectl -n build get pod -l job-name=build-proof -o jsonpath='{.items[0].spec.nodeName}'` — names a node in the target cluster, not the Mac. This is the check a local build plus a local push cannot fake.
- `crane delete docker.io/bborbe/agent-claude:proof` — the scratch tag is gone from the public repository. `clean` cannot do this; it removes only the local image.

## Desired Behavior

1. **One variable selects the machine.** `BUILD_TARGET` defaults to `local`; any other value is resolved by `hack/cluster-alias.sh` to a kubeconfig and context — `sun`, `nukedev`, `nukeprod`, or a raw context name — and the build runs as a Job there.
2. **The cluster arm renders a per-build Job.** `hack/build-cluster.sh` substitutes the repo URL, ref, tag, image, Dockerfile path, push secret, deadline and commit epoch into `hack/build-job.yaml`, refuses to apply when a Job of that name already exists, waits for it to succeed or fail, and relays the digest lines from the build log. `REF` is a required input on this arm and has no default.
3. **The local arm is reproducible.** `docker buildx build` writes through the `type=docker` exporter with `rewrite-timestamp=true,unpack=false`, with provenance and SBOM attestations disabled, so two builds of one commit yield one image ID. `BUILD_DATE` is the commit's date, not the wall clock.
4. **A remote build pushes and never loads.** The image exists in the registry and nowhere on the invoking machine, so `upload` and `clean` skip their local work when `BUILD_TARGET` is not `local`.
5. **The repo's own variables survive the port.** The image name comes from `IMAGE`, the tag from `VERSION`, and the clone URL from `https://github.com/$(IMAGE).git`; `build` keeps `check-version-tag` and `check-go-mod` as prerequisites.
6. **The image builds without a vendored tree, off a portable base registry.** The `go build` in the Dockerfile resolves modules from the module cache when `vendor/` is absent — the state of every cluster clone — and uses `vendor/` when it is present; the base-image default is `docker.io` rather than a machine-specific mirror host.

## Constraints

- **`hack/` is a verbatim copy of `bborbe/go-skeleton` at `18866d9d6b0d6159829f7a9e825232608067fee7`** — `build-cluster.sh`, `build-cluster-bootstrap.sh`, `build-job.yaml`, `cluster-alias.sh`, `dockerconfig.json.tpl`. No local edit. A repo-specific need is expressed in the Makefile variables the script reads, never in the script; that is what keeps this spec reusable for the next repo.
- **The container cannot read the go-skeleton checkout.** The dark-factory container mounts only this repo, runs with `.git` masked and has no Docker daemon, so the five files are fetched from `https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack/<file>` and the three shell scripts made executable. The pinned commit is what makes the copy reproducible; the operator-rung diff against that same commit is what proves it is byte-identical.
- **The reference's `BUILD_*` variables are declared in full** — `BUILD_TARGET`, `BUILD_NAMESPACE`, `BUILD_TIMEOUT`, `BUILD_REPO_URL`, `BUILD_DOCKERFILE`, `BUILD_PUSH_SECRET`, `REF`, `SOURCE_DATE_EPOCH`, `BUILD_DATE` — for parity with the reference, so this spec renders against the next repo unchanged. `build-cluster.sh` supplies defaults for four of them; declaring them here is what lets an operator override one without editing the script.
- **The push credential is named, never inlined.** `hack/dockerconfig.json.tpl` names a TeamVault lookup key; `hack/build-cluster-bootstrap.sh` resolves it with `teamvault-cli` into a `600` temp file, uses it for one `kubectl create secret --dry-run`, and removes it on every exit path.
- **`apply` is unchanged.** `DIRS` resolves to `cmd`, and `cmd/Makefile` includes `Makefile.folder`, whose own `DIRS` is empty — so `apply` recurses one level and runs no sub-target.
- **The local arm keeps its current cleaning behaviour** — `docker image prune -f`, `docker builder prune --all -f`, `rm -rf vendor` — so a local build cleans what it cleans today.
- **`check-go-mod` is retained even though the Dockerfile no longer requires `vendor/`.** It keeps the local arm's vendored build and `clean`'s `rm -rf vendor` unchanged, and this change does not reorganise module handling.
- **No `--pull`.** A moving base image would break the two-build image-ID equality this change establishes, and both base images are version-pinned (`golang:1.27.3`, `alpine:3.23`) except the final stage's `FROM alpine`, which is out of scope.
- **`--no-cache` is retained**, so a local build still recompiles rather than trusting a stale layer. `--rm=true` is dropped: buildx removes intermediate containers unconditionally, so it is a no-op there. `--platform=linux/amd64` is retained.
- **`SOURCE_DATE_EPOCH` needs no `ARG` in the Dockerfile.** BuildKit's dockerfile frontend reads it from the build-arg set to drive the exporter's `rewrite-timestamp`, which is what the two-build image-ID equality measures.

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
| The active buildx builder is not a `docker`-driver builder | `docker buildx build` writes to a separate cache, so the two-build image-ID equality does not hold | `docker buildx ls` and switch to the `*`-marked docker-driver builder |
| `VERSION` names a tag that is not `HEAD` and `ALLOW_UNTAGGED_BUILD` is unset | `check-version-tag` exits 1 naming both the HEAD tag and the requested version | Build from the tag, or set `ALLOW_UNTAGGED_BUILD=1` for a scratch build |
| `vendor/` is absent in the cluster clone | The Dockerfile's `go build` downloads modules instead of failing | None needed; this is the behaviour the change establishes |
| The pinned raw URL is unreachable from the container | The fetch step fails and no `hack/` file is written | Re-run the prompt; if it persists, the reference commit must be re-fetched by hand and the prompt updated |

## Security / Abuse

- The push credential exists only inside the cluster. `hack/dockerconfig.json.tpl` names a TeamVault key, the bootstrap script renders it into a `600` temp file for one dry-run and deletes it on every exit path, and the Job mounts it read-only at `/docker-config`. Nothing in the repo, the logs or the Makefile carries the value.
- The cluster clones the source repo anonymously. `bborbe/agent-claude` is public, which is why no git credential is needed inside the pod.
- The five reference scripts are fetched over HTTPS from a public repository at a pinned commit, so the fetch is reproducible and cannot be redirected by a branch moving.
- The scratch tag published during verification is public and must be deleted afterwards; `clean` removes only the local image.

## Suggested Decomposition

Prompts should be generated in this order — each row is a single prompt with a clear scope.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Vendor the five cluster-build scripts under `hack/` at their reference modes | 1 | 1, 2 | — |
| 2 | Dispatch `build` on `BUILD_TARGET`: the buildx local arm, the cluster arm, and the repo's variable names and prerequisites | 2, 5 | 3, 4, 5, 6, 7 | prompt 1 |
| 3 | Guard `upload` and `clean` for a remote target | 4 | 8 | prompt 2 |
| 4 | Build the image without a vendored tree, off a portable base registry | 6 | 9 | — |
| 5 | Document the build targets and record the change | 3 | 10, 11 | prompts 2, 3, 4 |

Rationale: prompt 1 lands the scripts the cluster arm calls, so prompt 2 can reference them without also authoring them. Prompt 3 depends on prompt 2's `BUILD_TARGET` block existing in the same file. Prompt 4 is independent. Prompt 5 records what the first four produced.

## Do-Nothing Option

`make build` keeps requiring the invoking machine's Docker daemon, and every release image of the worker that runs the fleet's agents stays tied to one Mac. The reference implementation exists and is proven on `go-skeleton`, so the cost of not doing this is not risk — it is that the fleet rollout has no worked example of the port on a repo whose variables and build context differ from the reference, and the next repo pays the same discovery cost this one would have paid once.
