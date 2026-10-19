---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-10-17T15:57:22Z"
generating: "2026-10-17T16:20:47Z"
prompted: "2026-10-17T16:20:47Z"
branch: dark-factory/a2a-worker-status-push
---

## Summary

- A docker worker built from this image stops being silent mid-turn: it pushes a status update while a turn is running and a completion when the turn ends, to a callback URL the spawner registered.
- The push uses the A2A protocol's own push-notification mechanism — the SDK's `CreateTaskPushNotificationConfig` registration (this SDK's JSON-RPC binding; the protocol's legacy name for it is `tasks/pushNotificationConfig/set`) plus the SDK's HTTP sender — so the wire shape is A2A's, not a bespoke `{worker_id, state, text, ts}` contract. That SDK method is named here on purpose: the exact standard binding is the requirement, not a detail an implementer may swap for a bespoke registration.
- The sender is SSRF-hardened by default and the spawner's callback is a same-host private address, so the worker must opt out of the private-network guard explicitly. Without that opt-out every push is refused and the channel silently does nothing — the trap this spec exists to pin.
- The worker advertises the capability in its Agent Card, so a spawner knows it may register a callback, and it fails closed: it never advertises push support it cannot actually deliver.
- Scope is the worker image only. The spawner-side receive and surfacing, and the controller-side push registration, are hand-implemented elsewhere and are not part of this work.

## Problem

A spawning session today cannot see what a worker is doing until the turn ends. The A2A conversation is pull-shaped: the spawner opens the turn and collects the reply, so every status fact is pulled at the end, never sent during. A worker that is stuck, slow, or working on the wrong thing is indistinguishable from a worker that is fine, for the whole duration of the turn — and turns run for minutes.

The A2A protocol already defines the mechanism that closes this: a client registers a push-notification callback for a task, and the agent pushes every task event to it. The SDK implements it, and this image's A2A surface does not wire it: the Agent Card advertises no push capability, so no client can register one, and no event is ever pushed. The capability the protocol provides is present in the dependency and absent from the deployed worker.

## Goal

A docker worker built from this image, in service mode, accepts a push-notification registration on its A2A endpoint and then pushes its task events to the registered callback — at least one mid-turn status update while the turn is running, and a terminal completion when it ends. The pushed payload is the A2A task-update shape, the registered token travels as the `A2A-Notification-Token` header, and the push reaches the spawner's same-host private callback address. A worker with no registration behaves exactly as it does today.

## Non-goals

- **The spawner side.** Receiving, validating, recording and surfacing the pushed status is `bborbe/claude-supervisor` work, hand-implemented from the parent task, not driven by this spec.
- **The controller side.** Registering the callback URL and minting the per-worker token on worker `create` is `bborbe/a2a-worker-controller` work, likewise hand-implemented.
- **The k8s arm.** A k8s pod cannot reach a Mac-local callback; the cluster arm is split to its own task. This spec is the docker arm only.
- **Changing how the spawner sends turns to a worker.** The request direction is untouched.
- **Streaming.** `capabilities.streaming` stays false. The mid-turn signal is a push, not a stream.
- Do NOT set `FailOnError: true` on the sender — a failing push must never fail the turn. Invariant; if a future consumer demands push-failure-fails-the-turn, that is a separate spec.
- Do NOT add a configurable mid-turn delay or a per-feature opt-out flag that disables the push — the delay is fixed and the channel is always on. Invariant; a knob with no named consumer is a regression, not a feature.
- Do NOT invent a bespoke callback contract, a bespoke token field, or a custom JSON payload shape — the A2A standard path is the requirement, not one option among several.
- Do NOT invent a custom handler that bypasses the SDK's task store to accept a pre-turn registration — the store's ownership check is the SDK's, and the spec's Constraints forbid replacing the SDK's own push path.

## Assumptions

- **The `github.com/bborbe/agent` companion lands first (or with this change).** Its `interactive` package is the extension point for the push store, the sender, the Agent Card capability and the executor's mid-turn event; without it the wiring described here is impossible and the feature is inert.
- **The spawner's callback resolves to a loopback or private address.** A container reaches the Mac host through the Docker gateway, which is a private address. A public callback would not need the private-network opt-out; the opt-out is required precisely because the legitimate target is private.
- **The spawner registers the callback only after the first `SendMessage` has created the task.** The SDK refuses a registration naming a task the task store does not hold, so a pre-turn registration is refused and the channel stays inert for that turn.

## Acceptance Criteria

- [ ] The served Agent Card advertises push-notification support. Evidence: `curl -s http://127.0.0.1:<port>/.well-known/agent-card.json | jq -e '.capabilities.pushNotifications == true'` exits 0. In the container rung, an integration test GETs the card handler and asserts the field rather than asserting a literal count.
- [ ] The worker accepts a push-notification registration for an existing task and returns it readable back, and refuses one naming a task it does not hold. Evidence: after the turn's first `SendMessage` has created the task, a JSON-RPC `CreateTaskPushNotificationConfig` call carrying a `{url, token}` for that task returns a success response and reading that task's configs back returns the same `url` and `token`; a call naming a task id the worker has never seen returns an error response and no config is stored for it.
- [ ] A single turn produces a mid-turn status update **and** a completion, both POSTed to the registered URL. The SDK sends one POST per task event, so a turn yields one POST per event — the executor's submitted-task, artifact and terminal-status events plus the new mid-turn status — and the two `statusUpdate` POSTs are selected by `status.state`, not by count. Evidence: a receiver bound to the registered URL records at least two POSTs for the task; among them the working-state `statusUpdate` has a receive timestamp falling strictly between the worker's own `turn start id=<id>` and `turn end id=<id>` log lines for that turn and a body carrying non-empty message text, and the completed-state `statusUpdate` has a receive timestamp at or after `turn end`.
- [ ] The pushed status lands mid-turn, not at a boundary. Evidence: on a turn made to run longer than 30 s, the mid-turn POST's receive timestamp is at least **9 s** after the `turn start` line **and** at least 10 s before the `turn end` line — a send fired at the last instant of the turn, or at its first instant, does not satisfy this. The 9 s floor, not 10 s, is deliberate and matches `bborbe/agent` spec 061's AC 7: `turn start` is emitted inside the `Prompt` goroutine while the delay timer is armed on the executor's goroutine, so the two instants are separated by sub-second scheduling skew and a 10 s floor fails intermittently — while 9 s still excludes an event emitted without waiting at all. The `turn end` clause stays at 10 s because it measures the opposite edge, where the delay works in the assertion's favour rather than against it.
- [ ] The push carries the registered token as the `A2A-Notification-Token` header, and carries no token header when the config has none. Evidence: positive — the receiver records a request header `A2A-Notification-Token` whose value equals the token registered; negative — a registration whose config omits the token produces a POST with no `A2A-Notification-Token` header at all.
- [ ] The push reaches a loopback/private target only because the private-network guard is opted out — the AC discriminates between the two configurations. Evidence: with the production sender configuration, a POST to a receiver bound to `127.0.0.1` succeeds and the receiver records it; with the default configuration (guard on), the same POST is refused and logged as a blocked target (`push notification target resolves to a blocked address range`) and the receiver records nothing — with `FailOnError` false the refusal is logged and swallowed, never returned to the caller. Evidence shape: positive receiver log + negative receiver log in the same run.
- [ ] The pushed body is the A2A stream-response union envelope, not a bespoke shape. Evidence: the received body parses as the SDK's `StreamResponse` union envelope whose single top-level key is the event discriminator — `statusUpdate` for the mid-turn and completion events — and whose value carries `taskId` and `status.state`; a body that is a hand-rolled `{worker_id, state, text, ts}` object, or one keyed on a non-existent `event` field, fails this.
- [ ] The worker fails closed: it never advertises push support it cannot deliver, and a registration is refused when push is not wired. Evidence: negative — with no push store and sender configured, the served Agent Card's `.capabilities.pushNotifications` is `false` and a registration call is refused rather than accepted-and-dropped.
- [ ] The existing A2A surface is unchanged. Evidence: `make test` exits 0; the existing route, authentication and session-cache contract tests pass unmodified. This spec changes neither the spawner (`bborbe/claude-supervisor`) nor the controller (`bborbe/a2a-worker-controller`) repository — neither checkout is mounted in this container, so that is an intent statement, not a runnable command here.
- [ ] `github.com/a2aproject/a2a-go/v2` is a direct dependency of this module. Evidence: `grep -n 'a2aproject/a2a-go' go.mod` prints a require line with **no** `// indirect` suffix, and `go mod tidy` leaves `go.mod` unchanged. This holds only because this repository builds the push store and the sender itself (see the seam in Constraints): with the store and sender constructed library-side, this module would import a2a-go indirectly at most.
- [ ] The mid-turn emission does not extend the turn. Evidence: on a turn whose work takes about T, `turn end − turn start` is closer to T than to `T + <the fixed mid-turn delay>` — a serial `sleep; yield; work` implementation, which pushes the turn's end out by the mid-turn delay while still landing inside the turn window, fails this.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` — exits 0.
- `make test` — the unit and integration suites pass.
- `grep -n 'AllowPrivateNetworks' main.go` — returns ≥1 line, and the value is `true`. The store and the sender are constructed in `main.go`, in the `newInteractiveService` function that builds the interactive service. This repository constructs the push store and the sender itself, which is what AC 10's direct-dependency requirement rests on.
- `grep -n 'a2aproject/a2a-go' go.mod` — a require line with no `// indirect`.
- `grep -n 'WithPushNotifications'` is **not** runnable in this repository and belongs to the `github.com/bborbe/agent` companion: that call site is the library's `interactive/a2a-handler.go`, which at `agent@v0.99.1` builds `a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(&a2aExecutor{cache: s.cache}))` with no options, so no line here matches it.
- The in-process integration test must first raise glog verbosity to 2 (`flag.Set("v", "2")`, as the companion's own `interactive/service_test.go` does): the `turn start id=<id>` / `turn end id=<id>` markers ACs 3 and 4 depend on are `glog.V(2)` lines (`interactive/session-cache.go`), so the mid-turn window is unverifiable in the test rung without it. The container rung is already covered — the image's entrypoint is `["/main", "-v=2"]` (`Dockerfile`).
- The integration test asserts, against a receiver bound to loopback: the card capability is `true`; a registration for an existing task round-trips while one naming an unknown task is refused; one turn yields a mid-turn status POST inside the turn window and a completion POST after it, without lengthening the turn; the token header is present and equals the registered token; and the same POST with the guard enabled is refused. These are behavioral assertions, not literal-count ones.

### Operator-executable (runs on the host — needs `docker`, no cluster)

- Build an image from the **current commit** and run it in service mode with a throwaway token and a public URL of its own: `docker build -t agent-claude-push-probe:$(git rev-parse --short HEAD) .` then `docker run` it. Building the tag from `HEAD` is the freshness assertion — a pre-existing `bborbe/agent-claude` tag is not acceptable evidence.
- Start a receiver on the host bound to `127.0.0.1`, register it as the push callback using `host.docker.internal` (which resolves inside the container to the Docker Desktop gateway), send one turn long enough to exceed the mid-turn window, and read the two POSTs off the receiver.
- Read the container log for the `turn start id=<id>` / `turn end id=<id>` pair and confirm the mid-turn POST's arrival falls inside that window. The window is measured against the worker's own clock, so no cross-host clock comparison is needed.
- The cluster worker in nuke dev runs the same image, but observing it is the **k8s arm**, which is out of scope here — do not run this probe against the cluster.

## Desired Behavior

1. **The Agent Card advertises push support.** The card the worker serves carries a true push-notification capability, so a client that reads the card learns it may register a callback. The capability is set from the same configuration that wires the push machinery — it is never advertised independently of it.

2. **A registration is accepted for a task the worker already holds.** The worker's A2A endpoint accepts a push-notification registration naming a callback URL and a token, stores it against the task it names, and returns it readable back. The SDK's handler refuses a registration naming a task its task store does not hold, so the spawner must register **after** the first `SendMessage` has created the task — there is no pre-turn registration and no config held for a task that does not yet exist.

3. **Every task event is pushed to the registered callback, in the A2A shape.** Each event a turn produces is delivered to every config registered for that task, through the SDK's own sender, so the payload is the A2A stream-response envelope and the registered token travels as the `A2A-Notification-Token` header. When the config carries no token, no token header is sent — an absent token is not an empty-string token.

4. **The push reaches the spawner's private callback.** The callback the spawner registers is a same-host address (a container reaches the Mac host through the Docker gateway, which is a private address). The sender is therefore built with the private-network guard explicitly relaxed, so the push is delivered rather than refused. The relaxation is scoped to that guard alone: the redirect cap and the cross-host token stripping remain in force, so a callback that redirects to a different host loses its token and a redirect chain is bounded.

5. **A turn emits a mid-turn status and a terminal completion.** While a turn is running the worker emits one status update — a working-state task-status event carrying a non-empty human-readable message — after a fixed delay from the turn's start, provided the turn is still running. When the turn ends the worker emits its completion, as it already does. Both events are pushed by rule 3, so a registered spawner sees the turn begin, sees it still running, and sees it finish.

6. **A push failure never fails the turn.** A callback that is unreachable, slow or returns a non-success status is logged and the turn continues to its normal outcome. The push is a reporting channel, not a step the turn depends on.

7. **An unconfigured worker advertises nothing and accepts nothing.** If the push store and sender are not wired, the worker does not advertise the capability and refuses a registration, rather than advertising a capability it cannot honour or accepting a config it will never deliver to. An unregistered worker behaves exactly as it does today.

8. **The existing surface is untouched.** Readiness, metrics, prompt intake, the permission endpoint, the Agent Card's other fields, the JSON-RPC binding, the session cache, per-session locking, the body cap and the authentication gate all keep their current behavior. The task-routed (Kafka job) path serves no HTTP and gains nothing.

## Constraints

- **Use the SDK's own push path — the wiring is `a2asrv.WithPushNotifications(store, sender)`, applied in the companion library to the store and sender this repository builds.** No bespoke callback endpoint and no bespoke event contract is built. The SDK's processor sends one push per task event, which is what makes a mid-turn status an ordinary task-status event rather than a new message type.
- **`AllowPrivateNetworks: true` is mandatory, and it is the decision that makes the channel work at all.** The sender refuses loopback, private, link-local and unspecified targets by default; the spawner's callback is exactly such an address, so without the opt-out every push is refused, the failure is logged only, and the channel is silently inert while looking deployed. The opt-out relaxes only the resolved-IP range check — the redirect cap and the cross-host token stripping still apply and must not be weakened further.
- **Auth is the push config's own token.** The SDK sends it as the `A2A-Notification-Token` header. There is no bespoke token field, and no new authentication mechanism is introduced.
- **The registration route inherits the service's existing authentication.** Registering a callback is a gated request, like every other non-exempt route; no new unauthenticated surface is added.
- **The store lives in the worker process.** The A2A standard puts the registration on the agent's own endpoint, so the worker holds its own per-task registrations. The store is not shared with the spawner or the controller, and no cross-process store is introduced.
- **The mid-turn delay is fixed, not configurable.** It is long enough that the status lands strictly inside a turn running longer than 30 s, and far enough from the start that it cannot be mistaken for the turn's first event. The value is a fixed constant, `midTurnStatusDelay = 10 * time.Second` — the delay itself, distinct from AC 4's *receipt* bounds, which are a 9 s floor (jitter-tolerant, see AC 4) and a 10 s ceiling; no env var, flag or config field exposes it.
- **`FailOnError` stays at its default (false).** A push failure is logged and swallowed; it must not cancel the turn.
- **Required companion change in `github.com/bborbe/agent` — this must land with or before this image.** The A2A handler and Agent Card are built inside that library's `interactive` package, which today constructs the handler with no options and the card with no push capability; the package exposes no way for a caller to supply a push store, a sender, or a capability decision. That package must be extended to accept the push configuration and to set the card's capability from it. The same package owns the executor's event sequence: `a2aExecutor.Execute` currently yields exactly submitted-task → artifact → terminal status, so the mid-turn working-state status event (DB 5) must be emitted there too — the seam is favourable, since that event slots between the first two yields, before the artifact update. **This repository cannot make either change** (it is a dependency, and a prompt in this repository targets this repository only), so the companion is hand-implemented or driven by its own spec in that repository, which has its own dark-factory instrumentation. Without it, the wiring described here is impossible and the feature is inert.
- **`docs/dod.md` applies.** A `## Unreleased` CHANGELOG entry and a README/doc note for the new push behavior are part of the change. The note must state the worker-side contract as domain knowledge that outlives this spec: the capability is advertised on the card, the registration is per task and requires the task to exist, the token travels as the `A2A-Notification-Token` header, the callback must be a private-network address (hence the opt-out), and the mid-turn status is one working-state update per turn. It sits beside the existing A2A run docs, `docs/local-docker-run.md` and `docs/cluster-a2a-run.md`.
- **No credential is logged.** The registered token, the interactive bearer token and every environment value stay out of the log and out of the pushed payload. The token travels in a header and nowhere else.
- **This repository changes only.** The spawner (`bborbe/claude-supervisor`) and the controller (`bborbe/a2a-worker-controller`) are not modified. Neither checkout is mounted in the YOLO container, so that is an intent statement here, not a runnable `git diff` check.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---|---|---|---|---|---|
| The registered callback is unreachable or refuses the connection | The push fails; the failure is logged; the turn continues to its normal outcome | None needed for the turn — the channel is reporting-only; restore the receiver and the next turn's pushes land | Worker log records the push failure while the turn still reaches its completion | Reversible — no state written | Two turns pushing concurrently are independent; each event is delivered per registered config |
| The private-network guard is left on (the opt-out missing) | Every push is refused as a blocked target; the turn is unaffected; the channel is inert while looking deployed | Set the opt-out; the next turn's pushes land | Worker log records the blocked-target error; the receiver records nothing while turns complete normally | Reversible — configuration only | n/a |
| The callback is slow or hangs | The push blocks the turn's event path until the sender's client timeout expires, then fails and is logged | None needed; a callback slower than the timeout is a callback-side defect | Turn duration grows by roughly the timeout; the log records the failure | Reversible | A hanging callback delays that turn only; other tasks' turns are unaffected |
| The registration names a callback that redirects to another host | The redirect is followed up to the cap and the token is stripped on the cross-host hop | None needed — this is the guard working | Receiver on the redirect target records a POST with no token header | Reversible | n/a |
| A client registers many configs for one task | Each event is delivered once per config, so outbound request volume grows with the registration count | Remove the surplus registrations | Receiver-side request count exceeds the event count | Reversible | Registrations for one task are stored together and all are used |
| The turn ends before the mid-turn delay elapses | No mid-turn status is emitted for that turn; the completion still is | None needed — the probe uses a turn long enough to exceed the delay | The receiver records one POST (the completion) and no mid-turn status | Reversible | n/a |
| The clock is skewed | The mid-turn window is measured against the worker's own `turn start` / `turn end` lines, which share its clock, so a skewed host does not break the check | Resync the host clock if the pushed event timestamps matter downstream | The window check still passes while a downstream consumer's timestamps look wrong | Reversible | n/a |
| Push is declared but the store or sender is not configured | The worker refuses the registration and does not advertise the capability — it never accepts a config it cannot deliver to | Wire both; the next registration is accepted | The card's capability reads false and the registration call is refused | Reversible | n/a |
| A registration names a task the worker has never seen | The SDK's handler finds no such task in its task store, so the registration is refused and no config is stored | Register after the turn's first `SendMessage` has created the task; the next registration is accepted | The registration call returns an error response and the receiver records nothing for that task | Reversible | n/a |

## Security / Abuse Cases

This feature adds an outbound HTTP POST to a **caller-controlled URL**, which is the textbook SSRF shape, plus a new authenticated registration route. Both need stating.

- **The callback URL is attacker-controllable if the registration is reachable.** The SDK's sender guards against exactly this by default, refusing loopback, private, link-local, unspecified and multicast targets — including cloud metadata endpoints — and re-validating at dial time so DNS rebinding and redirect hops are covered too. This spec deliberately relaxes that guard, because the legitimate callback is itself a private address. The relaxation is justified only by the registration being gated: only a caller holding the service's bearer token can register a callback, so the set of callers who can steer the destination is the set already trusted to drive the worker. The relaxation must not be widened beyond the resolved-IP range check, and the redirect cap and cross-host token stripping must stay in force so a registered callback cannot redirect the token to a third party.
- **The token is a credential and must not leak.** It travels as the `A2A-Notification-Token` header and nowhere else — not in the body, not in the log, not in the card. A cross-host redirect drops it. A registration that omits a token sends no header, rather than an empty one.
- **What can hang or retry forever.** A callback that accepts the connection and never answers holds the turn's event path until the sender's client timeout; a callback that redirects indefinitely is capped. Neither can retry unboundedly — the sender sends each event once and a failure is logged, not retried.
- **What must be validated.** The registration's URL is validated by the store before it is accepted, the registration is refused when it names a task the task store does not hold (so a caller cannot seed configs for tasks that do not exist), and the target address is validated by the guard at dial time. The registration inherits the service's existing session-id and body-cap validation; no new unvalidated input path is added.
- **No new unauthenticated surface.** The card stays public (it advertises no credential), and the registration route is gated by the existing bearer token like every other non-exempt route.

## Suggested Decomposition

| # | Repo | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|---|
| 1 | `github.com/bborbe/agent-claude` — this repository | Build the push store and the sender with the private-network opt-out in this repository, and pass both through `interactive.NewServiceWithPermissions` / `interactive.CardConfig` in `newInteractiveService` (`main.go`) — handing them to the SDK handler and setting the card capability is the library's job | 1, 2, 3, 4, 7 | 1, 2, 5, 7, 8, 10 | the `github.com/bborbe/agent` companion (see Constraints) |
| 2 | `github.com/bborbe/agent` — companion-side, not generated by this project | Emit the mid-turn status in the companion library's `a2aExecutor.Execute`: yield one working-state status update per turn after the fixed delay, while the turn is still running, slotted between the first two existing yields and without disturbing the submitted/artifact/completion sequence | 5 | 3, 4 | prompt 1 (companion-side, same seam) |
| 3 | `github.com/bborbe/agent-claude` — this repository | Integration test against a loopback receiver with a real sender: assert the mid-turn window, the completion, the turn not being lengthened, the token header (present and absent), the payload shape, and that the guard-on configuration is refused | 3, 5, 6 | 3, 4, 5, 6, 11 | prompts 1, 2 |

Rationale: prompt 1 makes the push possible at all and is where the two silent-failure traps live (the guard opt-out and the capability wiring) — it must land first and is worth auditing hardest. Prompt 2 adds the one event the existing sequence lacks, and depends on prompt 1 only for the delivery path it rides. Prompt 3 locks the wire behavior end-to-end, including the discriminating negative that proves the opt-out is load-bearing.

AC9 (the existing surface unchanged) is deliberately unmapped: it is a cross-cutting invariant that each prompt must preserve rather than one prompt's deliverable.

**Companion dependency.** The `github.com/bborbe/agent` extension point named in Constraints is a prerequisite, not a row: a prompt generated in this repository targets this repository only, so the library change is hand-implemented or driven by its own spec in that repository. Prompt 1 is written against the extension point's shape as fixed in Constraints, and prompt 2 against the executor's yield sequence as fixed there — prompt 2's deliverable is library-side (the mid-turn event lives in `a2aExecutor.Execute`), so it lands with the companion, not in this repository.

**Size note.** This is a 8-Desired-Behavior / 11-AC spec — above the 8×8 guidance when multiplied (88 > 50). It is left whole deliberately: the three prompts above are the natural seam, the behavior change is one coherent thing (a worker that pushes its own status), and the layers are shallow — one wiring point in this repository, one event added to an existing sequence, one test. Splitting would add an approval round without reducing what any single prompt must hold. Same posture as sibling 001.

**No new scenario.** The runtime-only failure this spec names — a push refused by the private-network guard, or delivered through Docker's gateway to a host listener — is reachable by the integration test's real sender against a loopback receiver for the guard half, and by the operator-executable rung for the Docker-networking half, which the verification ladder already runs. This repository has no `scenarios/` directory, and adding one would duplicate coverage the operator rung provides. Same rationale as siblings 001, 002 and 004.

## Do-Nothing Option

Do nothing and the worker stays silent until its turn ends. A spawning session keeps polling the reply or reading the worker's pane to learn what it is doing — which is the capability the parent goal exists to remove — and the A2A push mechanism stays present in the dependency and absent from the deployed worker, so every future consumer that wants worker-initiated status re-discovers the same gap. The cost of doing nothing is that a multi-minute turn is opaque for its whole duration, and the one protocol-standard fix for that stays unwired.
