# The Attention Store

The attention store is the operator channel a `claude-interactive` pod raises a
question or a permission gate on, and reads the answer back from. A pod has no
WezTerm pane and no spawning supervisor server, so neither of the carrier's two
existing delivery routes reaches it; the store is the one channel it does have.

This document records the store's contract as the pod depends on it. The store's
own implementation is **not** in this repository — the contract below is derived
from `scripts/pod-attention.py` and `scripts/answered-attribution.py`, which are
the pod-side halves and ship in the image. If the two disagree, the scripts are
the source of truth for the pod and this file is wrong.

The pod reaches the store **through `pod-attention.py`**, never through Go code.
`pkg/permission`'s gate shells out to the installed poster rather than
reimplementing the store protocol: the poster already encodes the `permission`
class, the `dedup_key` keying and the provenance rule, and a second
implementation would be a second thing to keep in step. Nothing in this
repository talks to the store directly.

## Classes

Every item carries an `answer_mechanism`, and that field is the class. The pod's
arm posts two:

| `answer_mechanism` | Raised by | How the answer comes back |
|---|---|---|
| `message` | `pod-attention.py ask` | a card the operator answers on the board |
| `permission` | `pod-attention.py gate` | a verdict the operator's arm delivers |

`permission` is the class a gate uses, and it is the class with the stricter
release rule below. A `message` item is read with the attribution rule in
`answered-attribution.py`; a `permission` item is read with `verdict()`. The two
are deliberately not the same rule.

An item also names its `producer_id` and a `producer_kind` from the store's
closed enum (`session | agent | cron | dark-factory` — there is no `pod`
member, and an unknown value is rejected with a 400 naming the field), plus a
`liveness_ref` (`<session|heartbeat|owner>:<value>`). ⚠️ `liveness_ref` decides
whether the card **survives**: the store prunes an open item whose producer it
cannot see live and that *asked* rather than reported, so a card posted by a
producer the store cannot resolve live is created and then removed on the very
first board read — it never reaches the operator. A pod's `gate` blocks on its
own item, so it is alive for as long as the gate matters; that is what the
`session:` model expresses.

## The `permission` release rule

A `permission` item releases its gate on exactly three conditions, verbatim from
`pod-attention.py`'s `verdict()`:

```
state == "answered" AND decision in {allow, deny} AND resolved_by is a non-empty string
```

The third condition is the whole legitimacy basis for releasing a pod's gate
without a pane: `resolved_by` is sourced from the answering arm's own
`CLAUDE_CODE_SESSION_ID`, and the board's JavaScript sends neither `decision` nor
`resolved_by`. **So no board click can release a gate — only an arm answer can.**
A `permission` card clicked on the board reads `answered` with no `resolved_by`
and the turn still runs out its timeout.

⚠️ This is the **`permission` class's** rule, not a store-wide one. It lives in
`pod-attention.py`'s `verdict()`, and it is the rule that mirrors
`permission-answer-poll.py` so the pod and the in-pane hook cannot disagree about
what releases a gate. The same `resolved_by` requirement was tried and
**withdrawn on 2026-09-27** for `answered-attribution.py`'s own predicate, which
governs `message`-class items: there it refused the operator's own board answer
and left the asking session frozen. The two files do not contradict each other —
they govern different classes.

## `dedup_key` keying

The store keys items by `dedup_key`. Two items must therefore never share one: a
key shared with another item hands this caller the other, already-answered item.
A second gate raised while one is open on the same producer gets its own item and
its own answer; neither inherits the other's decision. `pkg/permission`'s gate
mints a key from a process nonce plus an in-process counter, so the keys are
unique across both gates in one process and gates across restarts.

## Polling an item

`pod-attention.py poll ITEM_ID` prints one of **three** terminals, inherited from
`attention-ask.py` so the two arms cannot drift on what counts as an answer:

| Terminal | Meaning |
|---|---|
| `OPEN` | still unanswered |
| `ANSWERED:` | carries client evidence not positively flagged as automated |
| `NOT_OPERATOR_ANSWERED:` | moved on evidence the operator did not supply |

⚠️ **A caller gating on this must read `NOT_OPERATOR_ANSWERED:` exactly as it
reads `OPEN`**: the gate is not released. And `ANSWERED:` is **not** proof the
operator answered — a Playwright/CDP-driven Chrome reports
`navigator.webdriver === false`, so a scripted click stores `automation: false`
and prints `ANSWERED:` exactly as the operator's own click does. The boundary
lives in `answered-attribution.py`.

The payload of a gate names the tool, never the tool's input. The item is
readable by anything holding the store token, and the tool's input is not the
pod's to publish.

## Read surfaces

The store exposes two list surfaces, and the distinction decides what a count can
prove:

- `GET /api/1.0/attention` — the **open-only** list. An item that has been
  answered, closed or reaped is absent from it.
- `GET /api/1.0/attention/history` — **all states**, answered and closed items
  included.

⚠️ A count taken from the open-only list is **not** evidence that "nothing was
posted". An item that was posted and answered between two reads, or reaped,
leaves no trace on the open-only list. Any claim that a path produced no store
traffic must be read from `/api/1.0/attention/history`, which retains every
state. The store's own Go implementation is not in this repository, so neither
pod-side script can show you these routes; they are recorded here from the
store's API.

## Auth

Both the base URL and the token are read from the environment at run time and
never baked into an image or a manifest:

- `POD_ATTENTION_STORE_URL`, falling back to `ATTENTION_STORE_URL`, then to the
  local default `http://localhost:18080`.
- `POD_ATTENTION_TOKEN` — sent as `Authorization: Bearer <token>`, omitted when
  unset.

A **non-loopback** store with no token is **refused rather than attempted**: an
unauthenticated remote store lets anything that can reach the port read every
card and release every gate, which is strictly worse than the gap this closes.
The refusal is what stops an unauthenticated localhost default being read later
as the shipped shape. The token is a credential and must never appear in the
image, a manifest literal, a log line or a command line; `main.go`'s
`display:"length"` treatment of the field is the precedent.

## Fail-open, never fail-allow

Any error, malformed response or deadline prints `NO_DECISION:` and exits
non-zero **without** a verdict, so the gate stays for whatever fallback the pod
has. A default of allow would release a gate nobody approved; a default of deny
would refuse one nobody refused. Neither is safe to assume, so the poster has no
default — it never returns a decision it did not read. `pkg/permission`'s gate
inherits that rule: a poster that fails, refuses or prints no parsable verdict
denies, and an unanswered gate denies at its deadline rather than being released.
