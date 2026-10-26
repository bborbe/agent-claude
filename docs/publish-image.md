# Publish a Release Tag's Image

How a released `agent-claude` tag becomes a pullable image at
`docker.io/bborbe/agent-claude:<tag>`.

**This is currently a manual step, owned by the operator.** Agents get no Docker credentials,
so a worker or a CI job cannot do it. The fleet's automated route is in-cluster Kaniko fired
by `github-release-watcher`; this repo joins it as a consumer once that builder lands, and
this page becomes historical at that point.

A tag with no image is not a cosmetic gap: the cluster pulls the **mirrored** quant-registry
image, and the mirror is populated from `docker.io`, so a missing image leaves every pin bump
stuck and every pod pinning that tag in `ImagePullBackOff`.

## Prerequisites

- Docker running.
- `docker login` as the `bborbe` Docker Hub account. The credential is the operator's own —
  never store it in the repo, in a GitHub secret, or in a shell transcript.

## Procedure

The build is version-true: `make build` runs `check-version-tag`, which aborts unless HEAD
**is** the tag being built. Build from a checkout of the tag, never from a branch.

```bash
git fetch --tags origin
git worktree add --detach ../agent-claude-vX.Y.Z vX.Y.Z
cd ../agent-claude-vX.Y.Z

VERSION=vX.Y.Z make build upload
```

`make build` runs `go mod vendor`, builds `linux/amd64` from the repo's own Dockerfile, and
tags `docker.io/bborbe/agent-claude:vX.Y.Z`; `upload` pushes it. `DOCKER_REGISTRY` defaults
to `docker.io` in `Makefile:5` — do not override it, and do not use `make buca` (banned for
this repo, `CLAUDE.md` § Deploy).

Remove the worktree when done:

```bash
cd .. && git worktree remove agent-claude-vX.Y.Z
```

## Verify

Verify against the registry, never the git tag — a tag is not an image.

```bash
docker manifest inspect docker.io/bborbe/agent-claude:vX.Y.Z > /dev/null && echo present
```

Expect exit 0. Then confirm the artifact is that tag's own build rather than a stale image
retagged:

```bash
docker pull docker.io/bborbe/agent-claude:vX.Y.Z
docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' \
  docker.io/bborbe/agent-claude:vX.Y.Z
```

(`docker inspect` reads the local daemon, so the pull is what makes this check work from a
machine that did not do the build.)

Expect exactly `vX.Y.Z` — the label is stamped from `BUILD_GIT_VERSION`, which `make build`
feeds from `git describe --tags`.

Finally, confirm the toolchain the workers need is present:

```bash
docker run --rm --entrypoint git docker.io/bborbe/agent-claude:vX.Y.Z --version
```

Expect `git version …`.

## Build in the cluster

The local procedure above runs on the invoking machine and needs `docker login` there. The
cluster arm does not: `make build BUILD_TARGET=<cluster>` runs the build as a Kubernetes Job
in the target cluster, the Job mounts the push secret and pushes
`docker.io/bborbe/agent-claude` itself, so the registry credential never sits on the invoking
machine.

The two arms are equivalent in *where* they build, not in the bytes they emit. The Job passes
only the commit epoch, so a cluster-built image carries the Dockerfile's `dev` / `none` /
`unknown` version defaults while a local build stamps real values. That divergence is
recorded, not closed — do not read a cluster arm's missing version label as a build failure.

### Prerequisites

- `kubectl` with a kubeconfig for the target cluster.
- `crane`, to read the digest and to delete a scratch tag.
- `teamvault-cli`, logged in to the account holding the registry key.
- `docker login` is **not** needed on this arm; only the local procedure above needs it.

### One-time provisioning

Before the first cluster build against a cluster, provision its prerequisites:

```bash
hack/build-cluster-bootstrap.sh <cluster>
```

It is idempotent: it creates the `build` namespace and the `buildkit-push` Secret and exits 0
on a re-run. Re-running it is also the fix when the secret is missing, or when `buildkit-push`
was replaced with the reflector's `docker` secret — that one mirrors a different registry, and
a build mounting it fails at push with an error that reads like a build bug.

### Build

For a scratch build:

```bash
ALLOW_UNTAGGED_BUILD=1 VERSION=<scratch> REF=<branch> make build BUILD_TARGET=<cluster>
```

For a release build, the ref and the tag are the same:

```bash
VERSION=vX.Y.Z REF=vX.Y.Z make build BUILD_TARGET=<cluster>
```

`REF=` is required on this arm and has no default. `sun`, `nukedev` and `nukeprod` are
aliases; any other value is a raw `kubectl` context name.

**When it refuses.** A Job named `build-` plus the tag lowercased and sanitised, which already
exists, makes the script refuse and print its own delete command (`kubectl -n build delete job
<name>`); delete it and re-run. A `REF` whose cluster commit differs from the local one fails
the build container with both epochs named; run `git fetch` and re-run.

### Verify

The build prints a digest; the registry must return the same one:

```bash
crane digest docker.io/bborbe/agent-claude:<tag>
```

Then confirm the build actually ran in the cluster — the check a local build plus a local push
cannot fake:

```bash
kubectl -n build get pod -l job-name=build-<tag-lowercased> -o jsonpath='{.items[0].spec.nodeName}'
```

That names a node in the target cluster, not the invoking machine.

### Clean up a scratch tag

A scratch build publishes a public tag. Remove it when done:

```bash
crane delete docker.io/bborbe/agent-claude:<scratch>
```

`clean` cannot do this — it removes only the local image, and the cluster arm leaves none.

## After publishing

Publishing does not deploy. The nuke pin bump (`agent/values-dev.yaml` `tag:` **and**
`agent/Makefile` `MIRROR_IMAGES`) and the operator-gated `BRANCH=dev make apply` are a
separate step owned by the deploy path — see the vault's `Deploy Mirrored Agent Service`
runbook. A pin ahead of its image is exactly what puts `claude-interactive` into
`ImagePullBackOff`.

## Related

- [`local-docker-run.md`](local-docker-run.md) — building and running a scratch image locally
  while iterating; not a publish path.
- [`cluster-a2a-run.md`](cluster-a2a-run.md) — driving the deployed worker.
