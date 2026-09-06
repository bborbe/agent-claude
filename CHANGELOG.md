# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## v0.2.4

- chore: update Go to 1.27.1 and github.com/bborbe/agent to v0.87.1, github.com/bborbe/kafka to v1.25.13, github.com/bborbe/sentry to v1.10.1, github.com/bborbe/service to v1.10.13, github.com/bborbe/time to v1.27.14, github.com/bborbe/vault-cli to v0.122.4

## v0.2.3

- fix: update `golang.org/x/crypto` to v0.56.0 — clears the vulnerability gate that blocked this repo's CI
- fix: `Dockerfile` `ARG DOCKER_REGISTRY` default now points at `docker.prod.nuke.benjamin-borbe.de:443` instead of the decommissioned `docker.quant.benjamin-borbe.de:443`. Inert in CI (which passes `DOCKER_REGISTRY` explicitly) but a bare local `docker build` silently targeted a dead host

## v0.2.2

- chore: update github.com/bborbe/agent to v0.85.1, github.com/bborbe/cqrs to v0.6.10, github.com/bborbe/kafka to v1.25.11, github.com/bborbe/vault-cli to v0.121.0

## v0.2.1

- chore: update github.com/bborbe/agent to v0.85.0, github.com/bborbe/cqrs to v0.6.9, github.com/bborbe/errors to v1.6.0, github.com/bborbe/kafka to v1.25.10, github.com/bborbe/sentry to v1.10.0, github.com/bborbe/service to v1.10.10, github.com/bborbe/time to v1.27.11, github.com/bborbe/vault-cli to v0.118.4, github.com/onsi/gomega to v1.43.0

## v0.2.0

- feat: opt into `autoMerge.trivial` for mechanically-trivial update PRs

## v0.1.8

- chore: update github.com/bborbe/agent to v0.83.1, github.com/bborbe/cqrs to v0.6.8, github.com/bborbe/errors to v1.5.21, github.com/bborbe/kafka to v1.25.9, github.com/bborbe/sentry to v1.9.27, github.com/bborbe/service to v1.10.9, github.com/bborbe/time to v1.27.10, github.com/bborbe/vault-cli to v0.116.2

## v0.1.7

- chore: update Go to 1.27.0

## v0.1.6

- exclude no-fix docker/containerd advisories in checker config (GO-2026-4883/4887/5064/5338/5622/5932 v1 no-fix)
## v0.1.5

- fix: remove vestigial `include ../../common.env` from `cmd/run-task/Makefile` — leftover monorepo path that broke the recursive `make apply` sweep

## v0.1.4

- update Go to 1.26.6 and update dependencies (fixes GO-2026-6179, GO-2026-6180 in golang.org/x/mod)

## v0.1.3

- chore: update Go to 1.26.5, update dependencies, fix GO-2026-5841 in github.com/klauspost/compress

## v0.1.2

- fix(deps): bump x/text v0.39.0 (CVE-2026-56852) + Go 1.26.5 (GO-2026-5856); suppress unreachable/unfixable transitive CVEs (containerd, x/crypto/openpgp)

## v0.1.1

- refactor: converge build to bborbe/kafka-topic-reader publish-only model — make buca publishes docker.io/bborbe/agent-claude:$(VERSION); deploy machinery removed.

## v0.1.0

- feat: adopt cqrs v0.6.0 / agent v0.72.0 explicit `base.TopicPrefix`; add optional `TopicPrefix` config (`env TOPIC_PREFIX`) for Kafka result topic naming — empty means unprefixed topics (Octopus per-stage clusters), non-empty preserves `develop`/`master` names (quant)
- chore: bump `github.com/bborbe/agent` v0.70.0 → v0.72.0, `github.com/bborbe/cqrs` v0.5.2 → v0.6.0
