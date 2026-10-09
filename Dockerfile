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
ENV HOME=/home/claude
RUN mkdir -p /home/claude/.claude
ENV ZONEINFO=/zoneinfo.zip
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /
ENV BUILD_GIT_VERSION=${BUILD_GIT_VERSION}
ENV BUILD_GIT_COMMIT=${BUILD_GIT_COMMIT}
ENV BUILD_DATE=${BUILD_DATE}
ENTRYPOINT ["/main", "-v=2"]
