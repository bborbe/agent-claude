# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).


## Unreleased

- feat: install `openssl` in the `agent-claude` image — the `alpine` stage's `apk --no-cache add` line gains the binary so a `claude-interactive` pod can sign a JWT with an RS256 private key and print the signature. A GitHub App installation token is minted by signing a JWT with the App's private key and exchanging it at `POST /app/installations/<id>/access_tokens`, and every step of that exchange except the signature was already possible in the pod, which carries `curl` and `python3`; the image shipped no tool that could sign one, so the pod could not produce the signature even though it could perform the exchange. This is the binary alone — no credential, no key, no token-minting script and no git change — so a pod can sign before any credential exists, and the increment that wires a credential is a separate change


## v0.14.0

- feat: install `git` in the `agent-claude` image — the `alpine` stage's `apk --no-cache add` line gains the binary so a `claude-interactive` pod can clone a public repository into a directory under `/agent` and read it with its own `Read`/`Grep`/`Bash` tools. The image shipped no `git` at all, so the pod could not clone or read any repository: a credential would not have helped, because there was nothing to authenticate with. This is the binary alone — no credential, no push and no helper script — so a pod can read a public repository before any authentication exists, and the clone lands in the container's writable layer and is deliberately not persisted; the increment that authenticates over HTTPS is a separate change


## v0.13.1

- fix: bump `golang.org/x/net` to v0.60.0, with its `x/crypto`, `x/mod`, `x/sync`, `x/sys`, `x/term` and `x/text` transitives — the third gate behind the same required check, and the one that showed the earlier suppressions were aimed at the wrong layer. `trivy` reports these findings under **CVE** ids (`CVE-2026-97032`, `CVE-2026-78659`, `CVE-2026-78660`, `CVE-2026-78663`), not the `GO-` ids govulncheck uses, so the ids added to `.trivyignore` never matched them; the correct fix was the patch trivy itself named, `v0.58.0 → 0.60.0`. ⚠️ This is a **library** bump, not a toolchain bump, and the distinction is load-bearing: `go` stays at 1.27.1, because the `errcheck`/`golangci-lint` breakage that blocked the Go bump comes from the *toolchain* reading 1.27.2 export data, and a library bump touches none of that. Verified locally: `make lint` 0 issues, `make trivy` 0 vulnerabilities, `make osv-scanner` 0 packages affected with no unused ignores. Five of the thirteen advisories in `VULNCHECK_IGNORE` also carry an `x/net` line, but this bump does **not** retire them there — govulncheck reports each against the 1.27.1 stdlib as well, which was confirmed by removing them and watching `vulncheck` go red

- fix: bump `OSV_SCANNER_VERSION` to v2.6.0, which clears the second failure blocking the same required check — `panic: unexpected expr: *ast.KeyValueExpr`. Root-caused in a sibling session's PR #37, and credited here rather than absorbed silently: osv-scanner v2.3.1 pins `golang.org/x/tools v0.38.0`, whose SSA builder panics on a composite literal that keys a **promoted** struct field — a form Go 1.27 allows and the Linux stdlib uses at `internal/poll/splice_linux.go` (`&splicePipe{rfd: …, wfd: …}`). That file is GOOS-gated, so the panic fires only on Linux CI and every local macOS run of the same pinned version looks clean — which is exactly why it read as environment-specific until the gate was found. v2.4.0 is the first release carrying the fix; v2.6.0 is latest. ⚠️ `.osv-scanner.toml` is pruned to the five advisories v2.6.0 actually reports, because an entry that does not fire comes back as an unused ignore — the list is what the scan needs, not a mirror of `VULNCHECK_IGNORE`. The three containerd entries it used to carry no longer fire and were dropped with it

- fix: ignore the 13 `GO-2026-6599`…`GO-2026-6617` advisories that turned `make vulncheck` red on 2026-10-09, because their only fix is not adoptable in this repo. Every one of the 13 is fixed by Go 1.27.2, and moving to 1.27.2 breaks the check chain: `errcheck` dies with *"package sort without types was imported"* (export data version 5 > maximum supported 4) and `golangci-lint` needs ≥ v2.14.0 to read 1.27.2 export data. `golangci-lint` can move; `errcheck` cannot — v1.20.0 is its newest release, so no version reads 1.27.2 and the chain is stuck. The bump was tried and reverted rather than shipped half-working. The findings are not from this repo's code: they were published 2026-10-08T22:31Z, seven hours *after* master's last green run, and `go.mod` was untouched, so every open branch went red at once and master follows on its next run. It blocks more than a normal red check — `vulncheck` is one of the two required status checks on `master`, so while it is red no pull request in the repo can merge, whatever it changes. ⚠️ These are reachable stdlib `net/http` and `x/net` HTTP/2 paths, not unreachable transitives, so this is a suppression of live surface and not a free one. It is one line, reversible, and the comment above `VULNCHECK_IGNORE` names the exit condition: drop the ids and bump `go.mod` as soon as `errcheck` ships a Go 1.27.2-compatible release


## v0.13.0

- feat: teach the agent guardrails where a vault checkout lives and how to write to it — a new `## Vault` section in `agent/.claude/CLAUDE.md` states the `/agent/repos/<owner>/<repo>` convention (supplied by the deployment, not created by the agent), clone-on-demand, and the write policy: writes go to `master` because a vault's readers all read `master`, so an unmerged write is invisible to every one of them; `git pull --rebase` immediately before every push; on a rejected push re-read the remote version of the conflicted file, re-apply the intended change and retry once; on a second failure abort the push and raise the question to the operator rather than resolving it. It also forbids committing conflict markers into a vault file — an aborted push is the correct outcome — and excludes the daily note (`60 Periodic Notes/Daily/`), a vault's highest-collision file, which no task requires. Three rules harden the same path, all from the first review: the second failure runs `git rebase --abort` rather than leaving the checkout mid-rebase; `git push --force` is forbidden outright, because a vault has other writers and a force-push discards their work; and a credential must never be written into the checkout — not the remote URL, `.netrc`, `.git/config` or a committed file — since `No secret exfiltration` bans printing, logging and transmitting, not persisting to disk. ⚠️ One clause in `## Forbidden` is widened in the same change: public internet is now allowed for **the task's own repository remotes** as well as documentation and research, because a vault clone and push are neither. Nothing else is narrowed or relaxed — the ban on internal domains, cluster DNS and private IPs is untouched, the vault checkout is named as a designated output path rather than widening `## Data`, and **no vault is named**, so the image stays a template and the deployment names the checkout.


## v0.12.1

- fix: forward `POD_ATTENTION_STORE_URL` and `POD_ATTENTION_TOKEN` into the Claude child's environment — the pod's own env carries both, but the library replaces the child environment with a fixed allowlist (`HOME,PATH,USER,TZ,…`), so the poster running inside a turn never saw them and fell back to `http://localhost:18080`, where nothing listens inside a pod. The two names now ride `buildClaudeEnv`'s map, which the library applies as its final env layer — the seam `README.md` § Claude subprocess env allowlist already prescribes for exactly this case, so no `bborbe/agent` change is needed and no shared-library allowlist is widened. The failure is silent in the direction that matters: a turn that never needs the store completes normally, so the gap stays invisible until the first turn that must reach it. ⚠️ The token is read from the pod environment at runtime (`POD_ATTENTION_TOKEN` via a `secretRef`), so no credential becomes a literal in a CR or manifest; it is marked `display:"length"` and never reaches the startup log. Both names are omitted when unset, leaving a laptop run's child environment unchanged


## v0.12.0

- feat: ship the pod-side attention poster in the image — `scripts/pod-attention.py` and the sibling it loads by path, `scripts/answered-attribution.py`, are vendored verbatim from `bborbe/claude-supervisor` `scripts/` and copied to `/usr/local/bin/`, and `python3` joins the `alpine` stage's `apk add` because both are Python scripts and the image carried no interpreter. The poster is not a single file: it imports its sibling relative to its own directory (`_HERE`), so a copy that brings only the poster dies at import — before `argparse` runs, meaning even `--help` fails rather than degrading — which is why both are shipped and why a future sibling must be added here too. Until now the `claude-interactive` pod had no route by which to raise a question or a permission gate: the poster lived only in the supervisor plugin, which this image does not install, and nothing else in the pod spoke the attention store's protocol. The gap is silent in the direction that matters — a pod can serve a whole turn without ever needing a gate, so a missing poster stays invisible until the first turn that does, which is precisely the turn that then cannot reach the operator. `python3` is added to the `alpine` stage rather than to the final one, because the final `FROM alpine` resolves to that named stage — so the interpreter is installed where the runtime already is, not into a layer the image never copies from. The poster is baked in rather than fetched at runtime: a pod that pulled it over the network would depend on an egress path and an unpinned version, and could change behaviour under a running container. ⚠️ The copy is not a link — re-vendor it when the upstream script changes, or the pod keeps posting with an older protocol


## v0.11.1

- fix: bump `github.com/bborbe/agent` to v0.99.1 — the release evaluates the interactive service's session size limit (`interactive.DefaultMaxSessions`, 8) when a session is created as well as on the 30-second sweep, so a burst of callers arriving inside one window can no longer overshoot the maximum and grow the `claude-interactive` container until the kernel kills it; v0.99.0 enforced the limit only on the sweep, which left exactly that window open, because a session that keeps serving turns is never idle and the sweep only runs on its tick. No source change accompanies the bump: no shipped exported signature moved, so `main.go`'s `interactive.NewServiceWithPermissions` call site compiles unchanged. The same sessions are held and the same least recently used ones are dropped — what changed is when the bound is evaluated, not which sessions are kept


## v0.11.0

- feat: bump `github.com/bborbe/agent` to v0.99.0 and pass `interactive.DefaultMaxSessions` (8) as the interactive service's maximum session count — the release makes the constructors take a final `maxSessions` and enforce it on the allocation path, closing and dropping the least recently used conversation to make room; idle eviction alone bounded accumulation but not concurrency, because a session that keeps serving turns is never idle, so many simultaneous callers still grew the container until the kernel killed it. The deployment states the library default explicitly, and a non-positive value would be normalised back to it with a warning rather than disabling the limit. A caller returning after its conversation was dropped still gets a working session that has started over rather than an error, and the `claude-interactive` container's memory is now bounded by how many sessions are held at once, not only by how long they accumulate

## v0.10.0

- feat: bump `github.com/bborbe/agent` to v0.98.0 and pass `interactive.DefaultSessionIdleTimeout` (15 minutes) as the interactive service's session idle period — the release makes the constructors take a `sessionIdleTimeout` and evict a session that has gone that long without serving a turn, closing its conversation and dropping it from the cache; the deployment states the library default explicitly, and a non-positive value would be normalised back to it rather than disabling eviction. A caller returning after a long pause now gets a working session whose conversation has been rebuilt rather than the previous one, and the `claude-interactive` container's memory is bounded instead of growing with every distinct session id it has served

## v0.9.3

- fix: the `CLAUDE_ENV` and `ENV_CONTEXT` bags are no longer printed with their values by the startup configuration log — either can carry a credential (`ANTHROPIC_AUTH_TOKEN` is read from `CLAUDE_ENV` when the dedicated field is empty), and both were logged verbatim; each now renders as its sorted key names only, so the log still shows which variables a pod received

## v0.9.2

- fix: bump `github.com/bborbe/agent` to v0.97.1 — the interactive HTTP service now builds its server with a ten-minute `libhttp.ServerOptions.WriteTimeout` instead of the thirty-second `github.com/bborbe/http` default, so a turn that outlives thirty seconds can still write its answer instead of losing it to a write deadline that expired while the handler was still running (observed as `502 Bad Gateway`)

## v0.9.1

- fix: the Anthropic auth token is no longer printed in plaintext by the startup configuration log — `display:"password"` is not a value the argument printer honours, so the token fell through to the default branch and was logged verbatim; it now uses `display:"length"`, matching `INTERACTIVE_AUTH_TOKEN` and the Sentry fields

## v0.9.0

- feat: bump `github.com/bborbe/agent` to v0.97.0 (also pulls in the v0.96.1 result-deliverer fix that keeps an in-process-advanced phase across saves) and name the deployed agent in the A2A Agent Card — the card now advertises the value of `A2A_AGENT_NAME` instead of the shared library default, supplied through `interactive.CardConfig` alongside the public URL, and a service agent refuses to start without `A2A_AGENT_NAME` rather than advertising a generic name

## v0.8.0

- feat: bump `github.com/bborbe/agent` to v0.96.0 and wire the A2A public address — a service agent now serves the A2A Agent Card at `/.well-known/agent-card.json` and the A2A JSON-RPC endpoint at `/a2a`, and it refuses to start without `A2A_PUBLIC_URL` rather than advertising an empty or container-local endpoint; the address is bound through the `application` argument struct (not read from the environment ad hoc) and is passed to `interactive.NewServiceWithPermissions` as the advertised public URL, never derived from `LISTEN`; the task-routed job path is unchanged and does not need the address

## v0.7.0

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
