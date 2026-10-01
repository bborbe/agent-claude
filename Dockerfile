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
RUN apk --no-cache add ca-certificates curl bash nodejs npm \
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
ENV HOME=/home/claude
RUN mkdir -p /home/claude/.claude
ENV ZONEINFO=/zoneinfo.zip
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /
ENV BUILD_GIT_VERSION=${BUILD_GIT_VERSION}
ENV BUILD_GIT_COMMIT=${BUILD_GIT_COMMIT}
ENV BUILD_DATE=${BUILD_DATE}
ENTRYPOINT ["/main", "-v=2"]
