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
