ARG DOCKER_REGISTRY=docker.prod.nuke.benjamin-borbe.de:443
FROM ${DOCKER_REGISTRY}/golang:1.27.3 AS build
ARG BUILD_GIT_VERSION=dev
ARG BUILD_GIT_COMMIT=none
ARG BUILD_DATE=unknown
COPY . /workspace
WORKDIR /workspace
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -mod=vendor -ldflags "-s" -a -installsuffix cgo -o /main
CMD ["/bin/bash"]

FROM ${DOCKER_REGISTRY}/alpine:3.23 AS alpine
# Claude Code CLI is pinned so the stream-json protocol the interactive session
# implementation speaks cannot change under the image at build time.
# 2.1.286 is the current stable from the npm registry (`npm view @anthropic-ai/claude-code version`).
# `python3` carries the pod-side attention poster (scripts/pod-attention.py, vendored
# from bborbe/claude-supervisor). The poster is a Python script, so without an
# interpreter the image ships a file nothing can execute — and the failure surfaces
# only when a pod first tries to raise a gate, not at build time.
# `git` is installed so a `claude-interactive` pod can clone a public repository
# into a directory under `/agent` and read it with its own `Read`/`Grep`/`Bash`
# tools. This is the binary only: no credential, no push, no helper script — the
# increment that authenticates is a separate change.
# `openssl` is installed so a pod can produce an RS256 signature. A GitHub App
# installation token is obtained by signing a JWT with the App's private key,
# and the image shipped no tool that could sign one. This is the binary only:
# no credential, no key, no token-minting script — the increment that wires the
# credential is a separate change.
# `make`, `jq` and `util-linux` are the check prerequisites that are not Go: a
# repo's own `make check` needs `make` to run at all, `vulncheck` pipes through
# `jq`, and its failure path formats with `column` (util-linux — busybox has
# none). `gcompat` is the glibc shim `trivy`'s release binary needs on musl; see
# the toolchain block below. The Go toolchain and `trivy` are added there too.
RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git openssl jq make util-linux gcompat \
 && npm install -g --omit=dev --no-optional @anthropic-ai/claude-code@2.1.286 \
 && npm cache clean --force \
 && apk del npm \
 && rm -rf /root/.npm /tmp/*

FROM alpine
ARG BUILD_GIT_VERSION=dev
ARG BUILD_GIT_COMMIT=none
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.version="${BUILD_GIT_VERSION}"
COPY --from=build /main /main
COPY agent/ /agent/
# The pod-side attention poster and the sibling it loads by path, vendored verbatim
# from bborbe/claude-supervisor `scripts/`. It is how a cluster worker raises a
# question or a permission gate to the operator's attention board; `python3` is
# installed in the `alpine` stage above.
# ⚠️ The poster resolves its siblings relative to its own directory (`_HERE`), so every
# file it `_load`s must sit beside it — `answered-attribution.py` today. A missing one
# fails at import, before argparse runs, so even `--help` dies rather than degrading.
# ⚠️ Copied, not linked — re-vendor both when upstream changes, or the pod posts with
# an older protocol.
COPY scripts/pod-attention.py scripts/answered-attribution.py /usr/local/bin/
RUN chmod 0755 /usr/local/bin/pod-attention.py
# The git credential helper. It mints a GitHub App installation token from the
# environment and serves the git credential-helper protocol, so `git` obtains a
# token without the credential ever reaching a remote URL, a config file or a
# command line — `git` runs the helper itself and reads the answer from its
# stdout. The executable is named exactly `git-credential-github-app` because
# that is the name `git` looks for: there is no second executable and no alias.
COPY scripts/git_credential_github_app.py /usr/local/bin/git-credential-github-app
RUN chmod 0755 /usr/local/bin/git-credential-github-app
# Point `git` at the helper for github.com over HTTPS. This is a BUILD-time
# configuration, not a runtime one: agent/.claude/CLAUDE.md § Forbidden forbids a
# worker from modifying system config, so a worker cannot be asked to run
# `git config` itself — and a worker that forgets to set anything up cannot
# therefore operate unauthenticated-but-seemingly-fine.
#
# ⚠️ The value is the helper's ABSOLUTE PATH, and that is load-bearing. `git`
# prepends `git-credential-` to every value that is not an absolute path, so the
# obvious-looking `git-credential-github-app` resolves to
# `git-credential-git-credential-github-app` — a binary that does not exist. `git`
# then warns, falls back to a username prompt, and a non-interactive push dies.
# That is what shipped in v0.17.0, and the check that missed it read the stored
# value back (`git config --get-all …helper` prints the name) instead of resolving
# it, so it passed green against an unreachable helper. An absolute path is used
# verbatim, so the trap cannot be re-entered by editing the name.
# The `test -x` asserts the configured value really is an executable, so the image
# cannot build at all if the helper is missing or misnamed.
#
# `git` writes no credential here: /etc/gitconfig gains the helper's path only,
# and the token is minted per invocation and handed over on stdin/stdout.
RUN git config --system credential.https://github.com.helper /usr/local/bin/git-credential-github-app \
 && test -x "$(git config --get credential.https://github.com.helper)"
ENV HOME=/home/claude
RUN mkdir -p /home/claude/.claude
ENV ZONEINFO=/zoneinfo.zip
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /

# ── Source-repository build toolchain ─────────────────────────────────────────
# A cluster worker asked to carry a repo task must be able to run that repo's OWN
# `make check`. Every prerequisite except `trivy` is a `go run …` — lint, vet,
# errcheck, vulncheck, osv-scanner, gosec — so with no Go toolchain the check
# cannot run at all, and its failure is an ENVIRONMENT failure (`exit 127`,
# `command not found`) rather than a finding. Removing that state is the point.
#
# ⚠️ `go` is COPIED from the build stage, NOT `apk add`-ed, and that is load-bearing.
# The build stage is `golang:1.27.3`. This repo's own `Makefile.precommit` had
# recorded 1.27.2 as **not adoptable here**: `errcheck` v1.20.0 — then the newest
# release — could not read 1.27.2 export data, so the whole check chain stopped.
# 1.27.3 is adopted on 2026-10-21 because `GO-2026-6809` fails the vulncheck gate
# for every merge while `go.mod` sits on 1.27.1, and CI's `golangci-lint` chain
# reports 0 issues on 1.27.3 — so the errcheck hold does not extend to it. Copying the
# build stage's toolchain makes the pod's Go the SAME version the binary is built
# with, and makes that constraint impossible to drift past on an unrelated `apk`
# bump. `GOTOOLCHAIN=local` stops Go downloading a different one at run time, which
# would reintroduce exactly the version this pins away from.
#
# ⚠️ This is the SHARED image: every pod running `bborbe/agent-claude` gains this
# toolchain, not only `claude-interactive`. It is additive — nothing is removed —
# and it is the operator's own ruling (attention card
# `551f0d37d721f1963523cd505de4fcd0`, 2026-10-12) to fix the class, not the instance.
COPY --from=build /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOTOOLCHAIN=local

# `trivy` is the one check prerequisite that is not a `go run` — the repo's `trivy`
# target invokes the binary directly — and Alpine packages none, so it comes from
# its own release tarball. Pinned: an unpinned fetch would let the scanner change
# underneath a check whose result is recorded as evidence.
# ⚠️ `Linux-64bit` is the amd64 asset; this image is built for that platform.
# ⚠️ `trivy --version` runs at BUILD time on purpose. That binary is built against
# glibc and this stage is musl, so the failure mode to catch is "installs fine,
# cannot execute" — which would otherwise surface only when a pod first runs
# `make check`, as an environment-caused non-zero exit that reads like a finding.
# The `gcompat` package above is the shim for it; this assertion is what proves
# the shim actually worked, and it fails the build loudly if it did not.
ARG TRIVY_VERSION=0.75.0
RUN curl -sSfL "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz" \
      | tar -xz -C /usr/local/bin trivy \
 && test -x /usr/local/bin/trivy \
 && trivy --version

ENV BUILD_GIT_VERSION=${BUILD_GIT_VERSION}
ENV BUILD_GIT_COMMIT=${BUILD_GIT_COMMIT}
ENV BUILD_DATE=${BUILD_DATE}
ENTRYPOINT ["/main", "-v=2"]
