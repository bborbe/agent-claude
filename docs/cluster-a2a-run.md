# Cluster A2A Run

Hold a multi-turn A2A conversation with the deployed `claude-interactive` worker in the
nuke **dev** cluster, from a laptop session.

This is the cluster twin of [`local-docker-run.md`](local-docker-run.md). That recipe runs
the same client/worker path in a local container for a fast loop; this one drives the
deployed service through its ingress. The protocol facts and the traps are shared, the two
point at each other, and neither contradicts the other.

## What you are talking to

| | |
|---|---|
| Service | `claude-interactive-0`, namespace `dev` on **nuke dev** |
| Image | `bborbe/agent-claude`, `AGENT_TYPE=service` |
| Agent Card | `https://claude-interactive.dev.nuke.benjamin-borbe.de/.well-known/agent-card.json` — public, no token |
| Endpoint | `https://claude-interactive.dev.nuke.benjamin-borbe.de/a2a` |
| Auth | `Authorization: Bearer <token>` on every gated route |

## Prerequisites

- The **A2A MCP server** wired into your session as `a2a`, backed by
  `.claude/scripts/a2a-mcp-wrapper.sh` — see [Client](#client-the-vaults-a2a-mcp-server).
  If `a2a_agent_card` is not among your tools, the server is not wired: add it to the
  session's MCP config and reload.
- `kubectlnukedev` — the nuke **dev** cluster wrapper. Not `kubectldev` / `kubectlprod`
  (GKE, unrelated), and not `kubectlquant` (no longer carries these workloads).
- `teamvault-cli` configured for the personal config — the wrapper's cluster-token lookup.

## The Agent Card is the source of the endpoint

The card is public, so no token is needed. Call `a2a_agent_card()`, or fetch it directly:

```bash
curl -s https://claude-interactive.dev.nuke.benjamin-borbe.de/.well-known/agent-card.json \
  | jq -r '.supportedInterfaces[0].url'
```

Expect `https://claude-interactive.dev.nuke.benjamin-borbe.de/a2a`.

⚠️ **The card carries no top-level `url` field.** In the v2 SDK's A2A 1.0 shape the
advertised endpoint is `supportedInterfaces[0].url`, and a `jq -r .url` probe against a
correct card returns `null`. Never build a client that reads `.url`: it reports a dead
endpoint while the card is perfectly correct.

## Transport

JSON-RPC 2.0 over HTTP — `POST /a2a`, `method: "SendMessage"`, with the bearer token in the
`Authorization` header. One `SendMessage` is one turn.

⚠️ **The supervisor's `send_agent_message` is NOT this path.** It posts to `/prompt` with an
`X-Session-Id` header — plain text, a different route. The service registers `/prompt` and
`/a2a` as separate routes, and only `/a2a` reads its session from the request's `contextId`.
Anything reaching for `send_agent_message` has the wrong transport.

## Token

The bearer token is the **`INTERACTIVE_AUTH_TOKEN`** key of the **`claude-agent`** Secret in
namespace `dev`. It is never printed, never written to a file, never committed, and **no
value belongs in this document** — the key *name* only.

You do not read it yourself. `a2a-mcp-wrapper.sh` resolves it inside the server process, by
first match:

1. `A2A_TOKEN` already in the environment — an explicit caller override.
2. `A2A_BASE_URL` pointing at a local worker (`localhost` / `127.0.0.1`) — the local
   Keychain entry that `local-docker-run.md` writes.
3. Otherwise the cluster token from TeamVault key `VO0Gmw` — the same
   `INTERACTIVE_AUTH_TOKEN` the cluster names.

The token is passed via the environment only: never argv, never a file. Keep it that way. A
credential spliced into a command line is echoed by any shell error, which is why a session's
PreToolUse guards refuse both a direct Secret read and a `$(…)` credential lookup.

## Client: the vault's A2A MCP server

**This recipe uses the vault's canonical A2A MCP client, not raw `curl`.**

`.claude/scripts/a2a-mcp-wrapper.sh` launches `.claude/scripts/a2a-mcp.py` as an MCP server
named `a2a` over stdio. It exposes two tools:

- `a2a_agent_card()` — the remote card's name, endpoint and skill ids.
- `a2a_send(text, context_id="")` — sends one message and returns
  `{http_status, agent, task_id, context_id, state, reply}`.

Why this form: it resolves the token in-process, so the calling session never handles a
secret value, and its `A2A_BASE_URL` already defaults to the cluster host. It also reads the
endpoint from the Agent Card rather than from configuration, so a card change cannot silently
point the client somewhere else.

The raw `curl` form the original proof used is recorded under
[Appendix: raw curl](#appendix-raw-curl). It is what makes the `contextId` and the raw
JSON-RPC reply quotable, but it needs the bearer token in the calling session, which the
guards refuse. Prefer the MCP client.

## Two turns on one conversation

**`contextId` selects the conversation, and it MUST be identical on both turns.**

A first message sent *without* one runs under the service's default session — the literal
`identity`. So the natural probe (first message bare, follow-up carrying whatever
`contextId` the first reply returned) opens a **different** conversation on turn 2, and the
agent correctly reports no memory of turn 1. It reads like a memory bug and is a client bug.

**Choose the `contextId` up front and send it on both turns. Do not read it back off the
first reply.**

```
a2a_send(text="Remember the number 6184. Reply with exactly: ACK 6184", context_id="<CTX>")
a2a_send(text="What number did I ask you to remember? Reply with exactly: NUM <the number>", context_id="<CTX>")
```

`<CTX>` is any fresh UUID you pick before turn 1, e.g.
`python3 -c 'import uuid;print(uuid.uuid4())'`.

Expect turn 1 → `ACK 6184` and turn 2 → `NUM 6184`, both with `state: TASK_STATE_COMPLETED`
and both echoing the same `context_id`. Turn 2 referencing a value that appeared only in turn
1's prompt is the proof that the conversation continued.

## Verify

Server-side, on the pod:

```bash
kubectlnukedev -n dev logs claude-interactive-0 | grep -E 'turn (start|end)'
```

Expect **two** `turn start id=<CTX>` / `turn end id=<CTX>` pairs on the **one** id you chose —
not on `identity`, and not one id per turn.

**Default verbosity is enough.** `-v=2` is not required and is not evidenced; the markers are
in the default log.

If you see `turn start id=identity` for the first turn and `turn start id=<CTX>` for the
second, turn 1 was sent without a `contextId` — see above.

## Traps

- **Sessions expire.** A session closes after **15 minutes idle**, and the cache holds at
  most **8** sessions (LRU). Two turns meant to be one conversation must land inside that
  window, or the second turn rebuilds the session and the history is gone.
- **A turn blocks for its whole duration.** `SendMessage` is effectively synchronous, and the
  service's write deadline is ten minutes. The MCP client's HTTP timeout is 300 s, so a turn
  running longer than five minutes returns no answer to the client even though the service
  completes it. Keep probe turns short, or raise `TIMEOUT` in `a2a-mcp.py`.
- **The first reply's `context_id` is an echo, not an instruction.** It confirms the id you
  sent; it is not something to feed turn 2 if you sent none.

## Related

- [`local-docker-run.md`](local-docker-run.md) — the same client/worker path in a local
  container: fast loop, throwaway token, no cluster. Read it first when iterating on the A2A
  client itself.
- The Personal vault's `Agent2Agent (A2A) Protocol` page — protocol background, plus the
  `supportedInterfaces[0].url` and `contextId` findings recorded while proving this path.

## Appendix: raw curl

The form the original proof used. It makes the `contextId` and the raw JSON-RPC reply
quotable, but it needs the bearer token in the calling session:

```bash
CTX="$(python3 -c 'import uuid;print(uuid.uuid4())')"

send() {  # $1 = jsonrpc id, $2 = text
  curl -s --max-time 580 -X POST https://claude-interactive.dev.nuke.benjamin-borbe.de/a2a \
    -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":$1,\"method\":\"SendMessage\",\"params\":{\"message\":{\"messageId\":\"$(python3 -c 'import uuid;print(uuid.uuid4())')\",\"contextId\":\"$CTX\",\"role\":\"ROLE_USER\",\"parts\":[{\"text\":\"$2\"}]}}}"
}
```

`$TOKEN` must be the `INTERACTIVE_AUTH_TOKEN` value: read it in-process and never print it.
`--max-time 580` is sized against the service's ten-minute write deadline — the local
recipe's `280` is fine only because its container turns are fast.
