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

- Docker running, with the `buildx` plugin and a `docker`-driver builder selected
  (`docker buildx ls` marks it with `*`). The local arm builds through `docker buildx build`,
  so a Docker install without buildx fails with `docker: 'buildx' is not a docker command`,
  and a container-driver builder writes to a separate cache, which breaks the two-build
  equality described under the local procedure.
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

Expect exactly `vX.Y.Z` — the label is stamped from `BUILD_GIT_VERSION`, which the build feeds
from `git describe --tags` on either arm: `make build` on the invoking machine, and the
cluster Job from the cloned tree. **This check applies to a release build on either arm**, so a
`dev` result means the build did not reach the arm that produced this image, not that the check
is inapplicable. ⚠️ On the cluster arm it holds only when `REF` is the tag itself — see the note
under the cluster procedure.

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

The two arms are equivalent in *where* they build, and — for a release build — in the version
they stamp: both run the same `git describe --tags` derivation, and with `REF=<tag>` the
cluster arm's `org.opencontainers.image.version` reads `vX.Y.Z` exactly as the local arm's
does. ⚠️ **That equivalence is specific to a tag `REF`.** A `--depth=1` clone leaves the
checked-out commit parentless, so `git describe` can only resolve a tag pointing *exactly at*
HEAD; a branch or SHA `REF` falls through to `--always` and yields a bare short SHA, where a
local full-history build would yield `vX.Y.Z-N-gSHA`. `BUILD_GIT_COMMIT` and `BUILD_DATE` are
not forwarded either, so `.created` / `.revision` keep their defaults on this arm — no label is
affected here, because this image stamps only `.version`.

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

⚠️ **This arm passes `BUILD_GIT_VERSION`, so the Verify label check applies to a release build
on it.** The vendored `hack/build-job.yaml` derives the version from the cloned tree —
`git describe --tags --always --dirty` written to `/meta/git-version` — and forwards it as a
build-arg, so with `REF=<tag>` `org.opencontainers.image.version` reads `vX.Y.Z` rather than the
Dockerfile's `dev` default. The derivation is from the tree actually built rather than from
`__TAG__`, so the label cannot disagree with the commit it came from. ⚠️ **The fetch's `--tags`
is load-bearing**: without it the shallow clone carries no tag ref, `git describe` falls through
to `--always`, and the label becomes a short SHA — which fails the Verify check for a build that
is otherwise correct.

⚠️ **`REF=<tag>` is the only case the label assertion covers.** The scratch invocation above uses
`REF=<branch>`, and a `--depth=1` clone leaves the checked-out commit parentless, so `describe`
can only resolve a tag pointing exactly at HEAD — a branch tip ahead of the last tag falls through
to `--always` and the label is a bare short SHA, not `vX.Y.Z`. Do not read that as a build failure.

To exercise both, build the same tag from a tag ref and from a branch ref and compare the labels:

```bash
make build BUILD_TARGET=<cluster> REF=vX.Y.Z VERSION=vX.Y.Z-probe-tag
make build BUILD_TARGET=<cluster> REF=master  VERSION=vX.Y.Z-probe-branch
crane config docker.io/bborbe/agent-claude:vX.Y.Z-probe-tag    | jq -r '.config.Labels["org.opencontainers.image.version"]'
crane config docker.io/bborbe/agent-claude:vX.Y.Z-probe-branch | jq -r '.config.Labels["org.opencontainers.image.version"]'
```

Expect `vX.Y.Z` from the first and a short SHA from the second. Delete both scratch tags with
`crane delete` afterwards. ⚠️ **A Job named `build-<tag>` that already exists makes the script
refuse** — delete it first and re-run.

`BUILD_GIT_COMMIT` and `BUILD_DATE` are still not forwarded. This image stamps only
`org.opencontainers.image.version`, so no label is affected here; a repo whose Dockerfile
stamps `.created` / `.revision` would read `unknown` / `none` on the cluster arm.

Keep this file's copy of the manifest in step with `bborbe/go-skeleton`, which owns it
upstream — the fix belongs there first, not in a repo-local edit of a file pinned as a
verbatim copy.

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
