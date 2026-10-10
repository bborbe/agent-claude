# Local Docker Run

Run the A2A-enabled `agent-claude` service in a local container instead of deploying to the
cluster. Iterating on the A2A client/worker path — the Agent Card, `SendMessage`, and the
follow-up turn on one conversation — costs a container restart here instead of an image
publish, a mirror bump and a rollout.

To drive the **deployed** worker instead of a local container — the same client/worker path
through the cluster ingress — see [`cluster-a2a-run.md`](cluster-a2a-run.md).

This is a **development** recipe. It is not how the service ships: the cluster pulls the
mirrored image from the quant registry, and `make buca` is banned for this repo (see
`CLAUDE.md` § Deploy).

## Prerequisites

- Docker. On macOS (Docker Desktop, OrbStack, Rancher Desktop) `host.docker.internal`
  resolves automatically; on raw Linux `dockerd` add `--add-host=host.docker.internal:host-gateway`.
- The host's `claude-code-router` listening on `0.0.0.0:8788` — the container reaches it as
  `host.docker.internal:8788`. A `127.0.0.1`-bound router refuses those connections. See
  `claude-code-router` `docs/dark-factory-integration.md` § 1.
- The router's `allowedApiKeys` registry key, from `~/.claude-code-router/config.yaml`.
  **Read it into a shell variable at run time; never print it, never paste it into a file.**

## Build

Published tags exist for the cluster, but a local loop wants the image built from the tree you
are actually iterating on. `vendor/` must exist first — the Dockerfile builds with
`-mod=vendor`:

```bash
go mod vendor
```

Build natively for the host architecture. On Apple Silicon this is the fast path; the
`linux/amd64` build the Makefile produces is emulated and only needed for cluster parity:

```bash
docker build \
  --platform=linux/arm64 \
  --build-arg DOCKER_REGISTRY=docker.io \
  -t bborbe/agent-claude:local-scratch \
  -f Dockerfile .
```

`DOCKER_REGISTRY` defaults to the internal mirror (`docker.prod.nuke.benjamin-borbe.de:443`)
in the Dockerfile — override it to `docker.io` unless the laptop has mirror access.

For cluster parity instead, `ALLOW_UNTAGGED_BUILD=1 VERSION=scratch make build` runs the
repo's own target: same Dockerfile, `--platform=linux/amd64`, and it refuses to build unless
the tree carries the matching tag, hence `ALLOW_UNTAGGED_BUILD`.

## Run

```bash
export INTERACTIVE_AUTH_TOKEN="$(python3 -c 'import secrets;print(secrets.token_hex(24))')"
export ANTHROPIC_AUTH_TOKEN="$(python3 -c '
import yaml,os
d=yaml.safe_load(open(os.path.expanduser("~/.config/claude-code-router/config.yaml")))
print(d["allowedApiKeys"][0])')"

docker run -d --name agent-claude-local -p 9091:9090 \
  -e AGENT_TYPE=service \
  -e LISTEN=:9090 \
  -e A2A_PUBLIC_URL=http://localhost:9091/a2a \
  -e A2A_AGENT_NAME=claude-interactive-local \
  -e INTERACTIVE_AUTH_TOKEN \
  -e ANTHROPIC_BASE_URL=http://host.docker.internal:8788 \
  -e ANTHROPIC_AUTH_TOKEN \
  -e ANTHROPIC_MODEL=MiniMax-M2.7-highspeed \
  -e ALLOWED_TOOLS=Read,Bash,Grep,Glob,Write,Edit,WebSearch,WebFetch \
  bborbe/agent-claude:local-scratch
```

Pass credentials as bare `-e NAME` (no `=value`). Docker then reads them from the calling
shell's environment, so the value never reaches the container's command line, where `ps`
would expose it.

| Env | Why |
|---|---|
| `AGENT_TYPE=service` | Selects the long-running service shape. Without it the binary expects `TASK_CONTENT` and exits after one task. |
| `A2A_PUBLIC_URL` | The address the Agent Card advertises. It is configuration, **never derived from `LISTEN`** — set it to the URL the *client* reaches, including the `/a2a` path. Get it wrong and the service starts happily while every client POSTs to a dead endpoint. |
| `A2A_AGENT_NAME` | The name the card advertises. |
| `INTERACTIVE_AUTH_TOKEN` | Bearer token every gated route requires. A local throwaway is fine — it only has to match what your client sends. |
| `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` | The model router. Inside a container `127.0.0.1` is the container, so the host router is `host.docker.internal:8788`. The token is the router's registry key, which Claude Code sends as `Authorization: Bearer`. |

`AGENT_TYPE`, `INTERACTIVE_AUTH_TOKEN`, `A2A_PUBLIC_URL` and `A2A_AGENT_NAME` are fail-closed:
the service refuses to start without them rather than serving unauthenticated or advertising
an empty endpoint. `TASK_CONTENT`, `TASK_ID` and `BRANCH` are not needed in service mode;
`PROVIDER_BASE_URL` is optional (empty skips the readiness dial, which is also what the
cluster does).

### Pick a free host port

`9090` is commonly already taken on a laptop (`vault-ui` uses it). Publish on another port
and keep `A2A_PUBLIC_URL` consistent with it — the two must agree, because the client reads
its endpoint out of the card:

```bash
lsof -nP -iTCP:9091 -sTCP:LISTEN   # expect no output
```

## Verify

The card is public — no token needed:

```bash
curl -s http://localhost:9091/.well-known/agent-card.json
```

Expect `supportedInterfaces[0].url` to equal your `A2A_PUBLIC_URL` and `name` to equal your
`A2A_AGENT_NAME`:

```json
{
  "supportedInterfaces": [{ "url": "http://localhost:9091/a2a", "protocolBinding": "JSONRPC", "protocolVersion": "1.0" }],
  "name": "claude-interactive-local",
  "skills": [{ "id": "prompt", "name": "Prompt" }]
}
```

Note the endpoint is `supportedInterfaces[0].url`, **not** a top-level `url` — a
`jq -r .url` probe returns `null` against a correct implementation.

### Two turns on one conversation

`contextId` selects the conversation, and it must be **the same on both turns**. A first
message sent without one runs under the service's default session (`identity`), so a
follow-up that introduces a fresh `contextId` opens a *different* conversation and the agent
legitimately reports no memory of the first turn. Send the same id from the start:

```bash
TOKEN="$INTERACTIVE_AUTH_TOKEN"
CTX="$(python3 -c 'import uuid;print(uuid.uuid4())')"

send() {  # $1 = jsonrpc id, $2 = text
  curl -s --max-time 280 -X POST http://localhost:9091/a2a \
    -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":$1,\"method\":\"SendMessage\",\"params\":{\"message\":{\"messageId\":\"$(python3 -c 'import uuid;print(uuid.uuid4())')\",\"contextId\":\"$CTX\",\"role\":\"ROLE_USER\",\"parts\":[{\"text\":\"$2\"}]}}}"
}

send 1 "Remember the number 4271. Reply with exactly: ACK 4271"
send 2 "What number did I ask you to remember? Reply with exactly: NUM <the number>"
```

The second reply references the first (`NUM 4271`). Confirm it server-side — one `turn
start`/`turn end` pair per turn, both on the one session id:

```bash
docker logs agent-claude-local 2>&1 | grep -E "turn (start|end)"
```

The vault's A2A MCP client (`.claude/scripts/a2a-mcp.py`) drives the same path through its
`a2a_send(text, context_id=…)` tool; point `A2A_BASE_URL` at `http://localhost:9091` and set
`A2A_TOKEN` to the local `INTERACTIVE_AUTH_TOKEN`.

## The cluster heartbeat is already a no-op locally

There is no `claude-worker-heartbeats` ConfigMap and no service-account mount in a local
container. That is expected and needs no flag: `main.go` treats an unreachable cluster API
as "run without a heartbeat", logs a warning and serves anyway.

```
W main.go:333] cluster heartbeat disabled: create k8s config failed: unable to load in-cluster configuration, KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined
I service.go:200] starting http server listen on :9090
```

**Do not mount a kubeconfig into the container** to silence this. The heartbeat writer
resolves its namespace from the in-cluster service-account mount; a mounted kubeconfig can
let it attempt writes against an empty namespace instead of degrading.

## Gotchas

- **A turn blocks for its whole duration.** `SendMessage` is effectively synchronous — the
  A2A call returns only when the turn completes. The service's write deadline is ten
  minutes; a client timeout shorter than the turn loses the answer.
- **Sessions expire.** A session is closed after 15 minutes idle and the cache holds at most
  8 (LRU). Two turns meant to be one conversation must land inside that window, or the second
  turn rebuilds the session and the history is gone.
- **The startup log redacts the token** (`InteractiveAuthToken length 48`), but nothing else
  does. Never echo `INTERACTIVE_AUTH_TOKEN` or the router key into a shell transcript, a
  file, or a note.

## Related

- [`cluster-a2a-run.md`](cluster-a2a-run.md) — the same client/worker path against the
  deployed `claude-interactive` worker in nuke dev: the real token, the cluster ingress, and
  the pod-log verification. Same protocol facts, same traps; read it when you need the
  deployed service rather than a local container.
