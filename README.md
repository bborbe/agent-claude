# Agent Claude

**Reference implementation / copy-paste template** for Claude-based agents. Not a production agent itself — use this as the starting point when building a new domain-specific agent.

Generic, domain-agnostic Claude Code runner. Receives a task from the agent pipeline, spawns `claude --print` with configurable tools and instructions, and returns a structured JSON result.

New agents are created by swapping instructions (agent `.claude/CLAUDE.md`) and `ALLOWED_TOOLS` — no Go code changes needed.

## How It Works

1. Agent pipeline ([[task/controller]] → Kafka → [[task/executor]]) spawns a K8s Job with the `agent-claude` image.
2. The Job receives `TASK_CONTENT`, `TASK_ID`, `BRANCH`, `ALLOWED_TOOLS`, `MODEL`, etc. via env vars.
3. `main.go` assembles the prompt via `lib/claude` (embedded `workflow.md` + `output-format.md` + task content).
4. Runs `claude --print --output-format stream-json` with the allowed tools.
5. Parses the JSON result and publishes to Kafka via `lib/delivery.KafkaResultDeliverer` (when `TASK_ID` set), or falls back to `NoopResultDeliverer` for local runs.

### Service mode

With `AGENT_TYPE=service` the binary takes a second shape: instead of running one task and
exiting, it serves readiness, metrics and prompt intake on `LISTEN` (default `:9090`) and holds
one conversation per session id across requests. The HTTP surface comes entirely from the shared
`github.com/bborbe/agent/interactive` library — this repo adds no routing of its own. The executor
stamps `AGENT_TYPE=service` from the Config's `spec.type`; a service agent needs neither
`TASK_CONTENT` nor `TASK_ID`, and `PROVIDER_BASE_URL` is dialled for the readiness check. Every
route except readiness and metrics requires `Authorization: Bearer <token>`, and a service pod with
no `INTERACTIVE_AUTH_TOKEN` fails to start.

### Cluster heartbeat

A service agent publishes its own liveness so the fleet's cluster liveness reader can see it. Every
20 seconds it stamps one entry per session it has served within the last 90 seconds into the
`claude-worker-heartbeats` ConfigMap, in the namespace the pod runs in. The key is the session id
(the `X-Session-Id` the caller supplies); the value is `{"refreshedAt": "<RFC3339>"}`. The reader
(`scripts/cluster-heartbeat.py` in `bborbe/claude-supervisor`) treats a stamp older than 60 seconds
as dead, so a worker that stops being addressed ages out on its own with nothing to clean up. The
write merges into the ConfigMap's existing data, so two pods cannot erase each other's entries. A
failing write is logged and does not stop the service serving prompts. The pod needs RBAC to write
the ConfigMap — granted in the config repo, not here.

## Env Vars

| Var | Required | Default | Purpose |
|---|---|---|---|
| `TASK_CONTENT` | yes (unless `AGENT_TYPE=service`) | — | Raw task markdown |
| `INTERACTIVE_AUTH_TOKEN` | yes (when `AGENT_TYPE=service`) | — | Bearer token the interactive service requires on its gated routes; redacted from the startup log |
| `AGENT_TYPE` | no | — | `service` for a long-running identity agent; empty for a task-routed one |
| `LISTEN` | no | `:9090` | Readiness/metrics address (service agents only) |
| `PROVIDER_BASE_URL` | no | — | Provider endpoint a service agent dials for readiness |
| `BRANCH` | yes | — | `dev`/`prod` — used as Kafka topic prefix |
| `TASK_ID` | no | — | Required when publishing results via Kafka |
| `MODEL` | no | `sonnet` | `sonnet` or `opus` |
| `ALLOWED_TOOLS` | no | — | Comma-separated Claude tool allowlist (e.g. `Read,Grep,Bash`) |
| `AGENT_DIR` | no | `agent` | Directory containing `.claude/CLAUDE.md` guardrails |
| `CLAUDE_CONFIG_DIR` | no | — | Claude Code OAuth config directory (PVC mount) |
| `ENV_CONTEXT` | no | — | Comma-separated `KEY=VAL` pairs injected into the prompt |
| `CLAUDE_ENV` | no | — | Comma-separated `KEY=VAL` pairs passed to the Claude CLI subprocess |
| `KAFKA_BROKERS` | no | — | Required when `TASK_ID` is set |
| `SENTRY_DSN` | no | — | Error reporting |

## Creating a New Agent

To add a domain-specific agent that reuses this binary:

1. Create a task file in OpenClaw vault with `assignee: claude-agent` (or a new assignee routed to this image via a Config CRD).
2. Mount a PVC or Secret containing the domain-specific `.claude/CLAUDE.md` and any API credentials.
3. Set `ALLOWED_TOOLS` on the Config CRD to the minimum tools the agent needs.
4. Set `ENV_CONTEXT` to inject domain context (e.g. API URLs) into the prompt without modifying the binary.

### Config CRD env pattern

The `Config` CRD's `spec.env` map becomes pod env vars, which `main.go` consumes via struct tags. Example from `k8s/agent-claude.yaml`:

```yaml
spec:
  env:
    ALLOWED_TOOLS: WebSearch,WebFetch,Read,Grep
```

Tune `ALLOWED_TOOLS` per task shape (minimum viable set):

| Task shape | Minimum tools |
|---|---|
| Web research | `WebSearch,WebFetch,Read,Grep` |
| Vault I/O via scripts | `Bash(scripts/vault-read.sh:*),Bash(scripts/vault-write.sh:*),Bash(scripts/vault-list.sh:*),Grep` |
| API query via script | `Bash(scripts/trading-api-read.sh:*),Grep` |
| Code edit | `Read,Write,Edit,Grep,Glob,Bash(go:*),Bash(make:*)` |

Prefer constrained `Bash(path:*)` forms over bare `Bash` to minimize shell attack surface.

### Claude subprocess env allowlist

`lib/claude/claude-runner.go` strips pod env down to a safe allowlist (`HOME,PATH,USER,TZ,...`) before spawning `claude`. Custom env vars (API URLs, credentials) **must** be threaded explicitly via `ClaudeRunnerConfig.Env map[string]string` in `main.go`. Don't expect pod env to reach Claude by default. See `docs/` for precedent (trade-analysis commit `1ccfa674cf`).

## Local Quick Test

```bash
cd ~/Documents/workspaces/agent/agent/claude
go run . \
  --task-content "$(cat /path/to/task.md)" \
  --model sonnet \
  --allowed-tools "Read,Write,Edit,Bash,Grep,Glob" \
  --agent-dir agent \
  --branch dev
```

Skips K8s, task controller, task executor, git writeback. Useful for iterating on prompts.

## Links

Admin endpoints:
- Dev: <https://dev.quant.benjamin-borbe.de/admin/agent-claude/setloglevel/3>
- Prod: <https://prod.quant.benjamin-borbe.de/admin/agent-claude/setloglevel/3>

## Related

- `pkg/prompts/` — embedded prompts (`workflow.md`, `output-format.md`)
- `agent/.claude/CLAUDE.md` — default agent guardrails
- `docs/claude-oauth-setup.md` — seed PVC with Claude Code OAuth credentials
- `lib/claude/` — shared prompt assembly + Claude CLI invocation
- `lib/delivery/` — shared Kafka result publishing
- `task/controller/` — Obsidian→Kafka event source
- `task/executor/` — Kafka→K8s Job spawner
