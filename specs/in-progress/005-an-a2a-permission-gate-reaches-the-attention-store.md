---
status: prompted
approved: "2026-10-14T19:20:11Z"
generating: "2026-10-14T18:23:23Z"
prompted: "2026-10-14T19:54:54Z"
branch: dark-factory/an-a2a-permission-gate-reaches-the-attention-store
---

## Summary

- A permission gate raised during an A2A turn is answered on the pod's own `/permission` route, so the operator — who reads the attention board, not the panes — never sees it.
- The pod can already reach the attention store: the poster ships in the image and the store URL and token reach the process.
- The decider is the one component that does not use them, and it serves both routes from one instance.
- `bborbe/agent` now lets a decider tell the two routes apart, so this change can route the A2A path to the store while leaving the interactive path on `/permission`.
- Nothing about the interactive path's behaviour, the poster, the store, or the image changes.

## Problem

`bborbe/agent-claude` runs the A2A worker a manager spawns per task. When a turn calls a tool outside `--allowedTools`, the CLI raises a permission request and the pod's decider blocks on the pod's own `/permission` route. Measured live on 2026-10-14 against a k8s worker: the request appeared on `GET /permission`, the turn blocked 3 m 21 s, and a `POST /permission` verdict resumed it. **Nothing about that gate reached the attention board** — and in the same turn, the same pod successfully posted a message-class item to the store, so the poster, the store and the credentials are all demonstrably working.

The consequence is the one this row exists for: the operator answers from the attention board, and a gate that is only on `/permission` is invisible there. The turn runs out its timeout and denies.

The interactive path must not move. A `/prompt` gate is answered from the pane today, and its behaviour is the contract `bborbe/agent`'s `docs/interactive-service.md` freezes.

## Goal

A permission gate raised by an A2A turn is published to the attention store as a `permission`-class item, the turn blocks until the operator answers it there, and the answer resumes the turn — allow running the tool and deny refusing it. A gate raised by a `/prompt` turn keeps behaving exactly as it does today: it appears on `GET /permission` and resumes on a `POST /permission` verdict.

## Non-goals

- **Changing the attention store, its schema, or its auth.** This consumes the store as it stands.
- **Changing the poster.** `pod-attention.py` ships in the image and works. **The decider invokes it rather than reimplementing the store protocol** — the poster already encodes the `permission` class, the `dedup_key` keying and the provenance rule, and a second implementation would be a second thing to keep in step. Reimplementing the store call in Go is explicitly out of scope.
- **Changing the interactive path's behaviour**, its route, or `docs/interactive-service.md`'s existing contract rows.
- **Changing the A2A protocol** or the pod's HTTP routes.
- **Answering a gate from the board's JavaScript.** The store's release rule is class-dependent and the board cannot set `resolved_by` for a `permission` item.
- **Liveness.** The deployed store runs `SESSION_LIVENESS=off`; proving the pod's liveness ref is a separate row.

## Assumptions

1. **`bborbe/agent` releases the discriminator first.** `PermissionRequest.Source` does not exist at `v0.100.0`, which is what `go.mod` pins. This change cannot be built until that library releases and this repo bumps the dependency — the same sequencing the parent row's DoD records.
2. **The store URL and token are in the pod's own environment**, not only the child's. `POD_ATTENTION_STORE_URL` / `POD_ATTENTION_TOKEN` are read by `main.go` from the pod env, and the decider runs in the parent process, so it can read the same two variables directly.
3. **The two routes are distinguishable in `PermissionRequest`** — `Source` is `/prompt` or `/a2a`, set by the route that served the turn.

## Acceptance Criteria

- [ ] **Post-Deploy (Rung-2): AC1 — an A2A gate becomes a store item.** With the pod's store URL and token set, a turn whose tool call is outside `--allowedTools` produces exactly one new `permission`-class item on the attention store whose payload names the denied tool. Evidence: the item read back by id — `state`, `answer_mechanism` (`permission`) and the tool name in `payload` — recorded beside the turn's blocked state.
  - `deploy_check:` ``kubectlWRAPPER -n dev get pod <worker-pod> -o jsonpath='{.spec.containers[0].image}'``
  - `deploy_target:` the mirrored `agent-claude` tag carrying this change
- [ ] **Post-Deploy (Rung-2): AC2 — the turn blocks until the store answer arrives, and the answer resumes it.** The turn is still blocked while the item is unanswered, and completes **within 10 s of the item being answered** — the poster polls every 2 s (`DEFAULT_GATE_INTERVAL`), so a lag beyond 10 s is a fail, not a slow pass. Evidence: the item's `created_at` and `answered_at` beside the turn's start and end timestamps, with the answer→completion lag recorded and asserted against the 10 s bound.
  - `deploy_check:` ``kubectlWRAPPER -n dev get pod <worker-pod> -o jsonpath='{.spec.containers[0].image}'``
  - `deploy_target:` the mirrored `agent-claude` tag carrying this change
- [ ] **Post-Deploy (Rung-2): AC3 — deny refuses the tool.** The same sequence answered with a denial leaves the tool unrun and closes the turn on the denial. Evidence: the tool's absence from the turn's output and the item's `state: answered` with a deny decision.
  - `deploy_check:` ``kubectlWRAPPER -n dev get pod <worker-pod> -o jsonpath='{.spec.containers[0].image}'``
  - `deploy_target:` the mirrored `agent-claude` tag carrying this change
- [ ] **Post-Deploy (Rung-2): AC4 — fail-closed.** With the store unreachable, or the token absent, an A2A gate never resolves to allow. Evidence: a run with the store URL pointed at an unroutable address in which the turn ends without the tool running, and the pod log naming the failure.
  - `deploy_check:` ``kubectlWRAPPER -n dev get pod <worker-pod> -o jsonpath='{.spec.containers[0].image}'``
  - `deploy_target:` the mirrored `agent-claude` tag carrying this change
- [ ] **Post-Deploy (Rung-2): AC5 — the interactive path is unchanged.** A `/prompt` turn raising the same tool still appears on `GET /permission` and still resumes on a `POST /permission` verdict, and produces **no** store item. Evidence: `GET /permission` listing the pending id, the verdict resuming the turn, and the store's item count for that producer unchanged — read from `/api/1.0/attention/history` (all states), never from the open-only list.
  - `deploy_check:` ``kubectlWRAPPER -n dev get pod <claude-interactive-pod> -o jsonpath='{.spec.containers[0].image}'``
  - `deploy_target:` the mirrored `agent-claude` tag carrying this change
- [ ] **AC6 — the image builds and the suite passes.** `make precommit` exits 0.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

```bash
make precommit
make test
go test ./... -ginkgo.focus='permission'
```

These prove the decider's decision mapping, its error taxonomy and the compatibility
path. **They cannot produce AC1-AC5's evidence** — every one of those needs a live pod
turn against a live attention store.

### Operator-executable (runs on the host after the image lands in dev)

```bash
# AC1/AC2/AC3 — drive one A2A turn whose tool call is outside --allowedTools, then
# read the gate off the board and answer it. The pod is spawned per docs/cluster-a2a-run.md.
kubectlWRAPPER -n dev get pod <worker-pod> -o jsonpath='{.spec.containers[0].image}'
kubectlWRAPPER -n dev logs <worker-pod> --since=20m | grep -i 'permission\|attention'
# AC4 — repeat with POD_ATTENTION_STORE_URL pointed at an unroutable address; the turn
# must end without the tool running.
# AC5 — raise the same tool on the interactive pod and confirm GET /permission lists it,
# a POST /permission verdict resumes the turn, and the store count for that producer is
# unchanged (read /api/1.0/attention/history, all states — never the open-only list).
```

The deploy path is the Personal vault's `Deploy Mirrored Agent Service` runbook; this
repo's `CLAUDE.md` forbids `make buca` for this image, so `deploy_target` names the
mirrored tag on the dev workload, not a local build.

## Desired Behavior

1. The decider reads the raising route from the request it is handed and serves the two routes differently.
2. An A2A-raised request is published to the attention store as a `permission`-class item before the decider returns, so the gate exists while the turn is still open.
3. The decider blocks on that item and returns allow or deny from the operator's answer, never from a default.
4. An unanswered gate never resolves to allow. The decider holds until the store answers, and when the gate's own timeout expires (15 min, `DEFAULT_GATE_TIMEOUT`) it denies — the same terminal outcome the turn reaches today, by a different route.
5. A `/prompt`-raised request is decided exactly as it is today, with no store traffic.
6. The store URL and token are read from the pod's own environment at decision time, never baked into the image or a manifest.
7. A decider handed a request whose `Source` is empty behaves as it does today — a caller outside the interactive service, or a direct session test, is unaffected.

## Constraints

- **`github.com/bborbe/agent` must be bumped** to the release carrying `PermissionRequest.Source`; the version is re-confirmed at head rather than assumed, because the local checkout was 21 commits behind when this was written.
- **The interactive path's decider behaviour must not change.** `NewPermissionRegistry` keeps serving every non-A2A request; the new behaviour is added for one `Source` value.
- **No credential in the image, the manifest or a log line.** The token is read from the environment and never printed; `main.go`'s existing `display:"length"` treatment of it is the precedent.
- **The poster is already in the image** (`Dockerfile` copies `pod-attention.py` and `answered-attribution.py` to `/usr/local/bin/`). Do not re-vendor, re-point or duplicate it.
- **Do NOT commit.** dark-factory handles git.
- **No new dependency** beyond what the store call needs; `bborbe/agent`'s `interactive` package is already imported.
- **`docs/dod.md` applies** (this repo sets `validationPrompt: docs/dod.md`). A `## Unreleased` CHANGELOG entry and a `README.md` note are part of the change: `README.md` describes the pod as serving "the permission endpoint", and this change alters what that means for A2A turns.
- **The store contract must be recorded in this repo, not only consumed.** Nothing under `docs/`, `README.md` or `CLAUDE.md` currently describes the attention store — not the `permission` class, `answer_mechanism`, the class-dependent release rule that requires `resolved_by`, `dedup_key` keying, nor the `/api/1.0/attention/history` vs open-only-list distinction. Add that knowledge to `README.md` or a new `docs/attention-store.md`, because it outlives this spec.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection |
|---|---|---|---|
| The attention store is unreachable | The gate never opens; the turn ends without the tool running | Restore the store; the gate is re-raised on the next attempt. Never fall back to allowing | Pod log names the store failure |
| The token is absent or wrong | Same as unreachable — a 401 is not a verdict | Fix the Secret; the pod's fail-closed startup guard already covers a missing token at boot | Pod log carries the HTTP status, never the token |
| The store accepts the post but the item is never answered | The decider holds until the 15 min gate timeout, then denies; the item stays open on the board | Answer it on the board, or let it time out; the item is not silently closed | The item reads `open` past the turn's end; the turn ends on the timeout |
| A second gate is raised while one is open on the same session | Each gate gets its own item and its own decision; neither inherits the other's answer | None — the store keys items by `dedup_key`, and the decider polls its own item's id | Two open items for one producer |
| `Source` is empty (a caller outside the interactive service) | Today's behaviour — the registry decides | None; this is the compatibility path | No store item for that turn |
| The board's JavaScript answers a `permission` item | It cannot release the gate: the store requires `resolved_by` for that class, which only the arm sets. The item is bricked and runs out its timeout | Re-raise the gate; never click a pod's permission card on the board | The item reads answered with no `resolved_by`; the turn still times out |
| The pod's clock is skewed against the store's | AC2's lag arithmetic compares the store's `created_at`/`answered_at` with the pod's own turn timestamps, so a skewed pod clock can produce a negative or inflated lag and fail a correct implementation | Compute the lag from the pod's own log timestamps on both sides of the gate, or resync the clock and re-run | The recorded lag is negative, or differs from the pod's own log by more than the bound |

## Security / Abuse

The store token is a credential and must not appear in the image, a manifest literal, a log line or a command line. The decider reads it from the pod's environment at decision time. The `input_preview` the request carries is already described as "not logged" by `bborbe/agent`, and this change must not start logging it in order to build a store payload — the payload names the tool, not the tool's input. Publishing a gate is not a privilege escalation: the gate is already raised and already blocks; this only changes **where** the operator sees it and **which** channel answers it.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Bump `bborbe/agent`; add the store-publishing decider for the A2A `Source` and wire it beside the registry | 1-4, 6, 7 | 1-4, 6 | the `bborbe/agent` release |
| 2 | Prove the interactive path unchanged — the `/prompt` gate still on `/permission`, no store traffic | 5 | 5 | prompt 1 |

Rationale: prompt 1 lands the behaviour and its failure modes; prompt 2 is a separate, negative-evidence check on the path that must not move, which is the half a single prompt is most likely to leave unproven.

**No new scenario.** This repo has no `scenarios/` directory. The runtime-only failure — a
gate that exists but is invisible to the operator — is reached by the operator-executable
ACs above, against a real pod and the real store; a fake store server would cover the
decision mapping (already unit-tested) and none of the block-and-resume path, which is the
part that fails. Adding a scenario would duplicate the live ACs at the cost of a harness.

## Do-Nothing Option

The A2A worker keeps answering its own gate on `/permission`, where the operator never looks. The parent row's Success Criteria stay unmet: a real gate from a worker does not reach the board, and the answer does not resume the worker from there. The workaround — a manager polling each pod's `/permission` route — puts a human-facing decision behind a route with no board presence, no provenance record and no answer history, which is the opposite of what the attention store exists for.
