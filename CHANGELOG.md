# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## Unreleased

- feat: a service agent publishes its own cluster liveness — every 20 seconds it stamps one entry per session it has served within the last 90 seconds into the `claude-worker-heartbeats` ConfigMap (key: the session id, value: `{"refreshedAt": "<RFC3339>"}`), so the cluster liveness reader in `bborbe/claude-supervisor` returns a live cluster worker while it is served and stops returning it within 60 seconds of its last refresh, and a session's first entry is stamped the moment it is served; the write merges into the ConfigMap's existing data, a failing write is logged without taking prompt serving down, and the pod's own namespace is resolved from the service-account mount rather than configured
- feat: add `pkg/heartbeat` — an `ActivityRecorder` that remembers recently-served session ids within a 90s idle cutoff, a `ConfigMapWriter` that read-modify-writes one `refreshedAt` entry per session into the `claude-worker-heartbeats` ConfigMap so a write merges with other writers' keys instead of replacing them, a `Publisher` that re-stamps active sessions every 20s and logs-and-continues on a failed write, and a `NewObservingSessionFactory` decorator that records a session id when its `Prompt` returns; nothing is wired into the running service yet

## v0.6.0

- feat: the interactive service now requires a bearer token on every gated route — a service agent refuses to start without `INTERACTIVE_AUTH_TOKEN` instead of serving its prompt-intake and permission routes unauthenticated; the task-routed job path is unchanged and does not need the token; the token is delivered as a runtime-only pod secret and its value is redacted from the startup config log
- chore: update github.com/bborbe/agent to v0.94.0 — the library adds the required `auth` parameter to `interactive.NewService` and `interactive.NewServiceWithPermissions` and the bearer-token gate that consumes it

## v0.5.0

- chore: update github.com/bborbe/agent to v0.93.1 — the library now passes `--permission-mode manual` to the Claude session process whenever a decider is wired. Without it the CLI ran in its default `auto` mode, which approves a tool invocation without asking, so the decider was never consulted and `GET /permission` stayed empty however many turns ran. With the flag a tool call outside the allow set pauses the turn and appears on the endpoint, which is what the service mode exists to expose.

## v0.4.0

- feat: wire the service mode to the permission endpoint — `factory.CreateClaudeSessionFactory` takes the caller's `interactive.PermissionRegistry` and passes it where it previously passed a literal `nil`, and `runService` constructs one registry and hands the same instance to both the session factory and `interactive.NewServiceWithPermissions`, so a turn that pauses on a tool permission is observable on `GET /permission` and releasable by `POST /permission`
- chore: update github.com/bborbe/agent to v0.93.0 — pulls in the permission registry, the `NewServiceWithPermissions` constructor and the `/permission` routes

## v0.3.0

- feat: `agent-claude` gains a service mode — with `AGENT_TYPE=service` the binary serves readiness, metrics and prompt intake on `LISTEN` (default `:9090`) via the shared `github.com/bborbe/agent/interactive` service instead of running one task and exiting, holding one conversation per session id; the task-routed job path is unchanged
- feat: add `factory.AgentTypeService` (the `AGENT_TYPE` value the executor stamps from a service Config's `spec.type`) and `factory.CreateClaudeSessionFactory`
- fix: `TASK_CONTENT` is no longer required at argument-parse time and `TASK_ID` is a plain `string` — a service agent carries neither, and `TaskIdentifier.Validate` rejected the empty id before the pod could reach the service branch
- chore: update github.com/bborbe/agent to v0.92.0 — pulls in the `interactive` service and the Claude streaming session
- chore: pin `@anthropic-ai/claude-code` to `2.1.286` in `Dockerfile` so the stream-json protocol the session implementation speaks cannot change under the image at build time
- fix: depguard's `github.com/bborbe/argument` deny rule now matches v1 exactly (`$` suffix). Prefix matching also denied `github.com/bborbe/argument/v2` — the package its own description says to use — so any file importing it failed `make lint`

## v0.2.6

- chore: update github.com/bborbe/agent to v0.89.0 — pulls github.com/bborbe/errors to v1.6.1, github.com/bborbe/kafka to v1.25.16 and github.com/bborbe/vault-cli to v0.126.3
- chore: exclude `k8s.io/kube-openapi v0.0.0-20260904170622-9ab3195f2a72` — it pulls `sigs.k8s.io/structured-merge-diff/v7`, which collides with the `v6` the k8s v0.37.0 stack uses and fails to compile in `k8s.io/apimachinery@v0.37.0`

## v0.2.5

- fix: `make build` refuses to stamp a version onto a tree that is not that version's tag (`check-version-tag`, escape hatch `ALLOW_UNTAGGED_BUILD=1`). `VERSION` defaults to the newest tag repo-wide, so an operator-run build from an untagged or older tree silently republishes under the newest tag. The guard compares `git describe --exact-match HEAD` against `$(VERSION)` and exits non-zero on mismatch.

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
