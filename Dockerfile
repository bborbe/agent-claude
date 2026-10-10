ARG DOCKER_REGISTRY=docker.prod.nuke.benjamin-borbe.de:443
FROM ${DOCKER_REGISTRY}/golang:1.27.1 AS build
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
RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git openssl \
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
ENV BUILD_GIT_VERSION=${BUILD_GIT_VERSION}
ENV BUILD_GIT_COMMIT=${BUILD_GIT_COMMIT}
ENV BUILD_DATE=${BUILD_DATE}
ENTRYPOINT ["/main", "-v=2"]
