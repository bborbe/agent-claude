---
status: approved
spec: [005-an-a2a-permission-gate-reaches-the-attention-store]
created: "2026-10-14T19:44:38Z"
queued: "2026-10-14T20:22:06Z"
branch: dark-factory/an-a2a-permission-gate-reaches-the-attention-store
---

<summary>
- Proves, with tests, that the interactive `/prompt` path did not move when the A2A gate was added.
- A gate raised by a `/prompt` turn still lands on the pod's own permission route, and still reaches nothing else.
- The negative evidence is explicit: the attention store poster is never invoked for a `/prompt` turn, and the test fails loudly if it ever is.
- A caller that names no route at all is covered by the same proof.
- The pod's `GET /permission` route is exercised end to end and still lists a pending gate raised the `/prompt` way.
- The attention store's contract — its classes, its release rule and its read surfaces — is written down in this repo instead of living only in a script's docstring.
- The README points at that contract.
- The CHANGELOG records the documentation.
</summary>

<objective>
Prove that the interactive permission path is unchanged by the A2A gate work — a `/prompt`-raised request still resolves through the pod's own permission registry and produces no attention-store traffic — and record the attention store's contract in this repo so it outlives the spec.
</objective>

<context>
Read `docs/dod.md`. Read the coding guides at `/home/node/.claude/plugins/marketplaces/coding/docs/`: `go-testing-guide.md`, `go-patterns.md`, `go-error-wrapping-guide.md`, `changelog-guide.md`, `go-doc-best-practices.md`.

**Depends on prompt 1** (`prompts/1-spec-005-a2a-gate-decider.md`), which must already be merged. Prompt 1 creates `pkg/permission` with `NewDecider(registry claudelib.PermissionDecider, gate Gate) claudelib.PermissionDecider`, the `Gate` interface with method `Decide(ctx, request) claudelib.PermissionDecision`, `NewGate(posterPath, producerID string) Gate`, and the counterfeiter fake `mocks.PermissionGate`. If `pkg/permission` does not exist, stop and return `Status: failed` with the message `"pkg/permission not present — prompt 1 (1-spec-005-a2a-gate-decider) has not shipped"`. Do not create it here.

### Why this prompt exists

The spec splits deliberately: prompt 1 lands the A2A behaviour and its failure modes, and this prompt is the **negative-evidence** half — the check on the path that must not move, which a single prompt is most likely to leave unproven. The whole risk of this change is that the routing decider, which now sits in front of the registry for **every** session, accidentally diverts a `/prompt` gate to the store or blocks it on a store the interactive pod cannot reach.

### Verified facts

`claude.PermissionRequest.Source` is `claude.PermissionSource` (`type PermissionSource string`), with `claude.PromptPermissionSource PermissionSource = "/prompt"` and `claude.A2APermissionSource PermissionSource = "/a2a"`. The empty value is what a session whose caller named no route sees.

`interactive.NewPermissionRegistry()` returns `interactive.PermissionRegistry`, which satisfies `claudelib.PermissionDecider` and additionally offers `List() []interactive.PendingPermission` (never nil) and `Resolve(id string, decision claude.PermissionDecision) bool`. Its `DecidePermission` "registers request as pending and blocks until a verdict is posted for it or ctx is cancelled" — so a spec that calls it must cancel its context to return.

`interactive.PendingPermission` is `{ID string \`json:"id"\`; ToolName string \`json:"tool_name"\`; Description string \`json:"description"\`; InputPreview string \`json:"input_preview"\`}`.

`interactive/service.go`'s `Handler()` registers `router.Handle("/permission", s.permissionHandler())` **only when the service was built with a non-nil registry**, then wraps everything in `s.requireAuth(router)`. `GET /permission` answers with `json.Marshal(s.permissions.List())` — a JSON array, and exactly `[]` when nothing is pending.

`interactive/auth.go` verifies from source: the header is `Authorization`, the scheme is `Bearer ` (with the trailing space), and the comparison is a constant-time compare against the configured token. `authExempt` exempts only `/readiness`, `/metrics` and `/.well-known/agent-card.json` — **`/permission` is gated**, so a spec that calls it must send `Authorization: Bearer <the token the service was built with>`.

`main_internal_test.go` is `package main` (the `application` struct is unexported) and already contains a `Describe("application.newInteractiveService")` block that serves the real Agent Card via `service.Handler()` + `httptest.NewRecorder()`. Its imports already include `interactive "github.com/bborbe/agent/interactive"`, `agentmocks "github.com/bborbe/agent/mocks"`, `net/http`, `net/http/httptest` and `github.com/prometheus/client_golang/prometheus`.

`scripts/pod-attention.py` and `scripts/answered-attribution.py` are the source of truth for the store contract. Both are already in the repo — read them; do not invent schema.
</context>

<requirements>
1. **Negative evidence: the poster is never invoked for a `/prompt` request.** In `pkg/permission/decider_test.go` (created by prompt 1) add a spec that builds the decider with the **real** `interactive.NewPermissionRegistry()` and a gate pointed at a fake poster shim which, if it ever ran, would create a marker file in `GinkgoT().TempDir()`:

   ```sh
   #!/bin/sh
   : > "$MARKER"
   echo "DECISION: allow -- should never run"
   exit 0
   ```

   Pass the marker path through the shim's environment (or write the path into the script literally — either is fine, but the marker must be inside the temp dir so it is cleaned up). Build the **production routing** rather than calling the registry directly — `gate := permission.NewGate(<the shim's path>, "test-producer")` and `decider := permission.NewDecider(registry, gate)` — then run `decider.DecidePermission` on a request with `Source: claudelib.PromptPermissionSource` in a goroutine, cancel its context after the assertion, and assert:
   - the registry reports the request as pending — `Eventually(registry.List()).Should(HaveLen(1))` and its `ToolName` is the one raised;
   - the marker file **does not exist**, asserted with `Consistently(...).ShouldNot(...)` across a window long enough to have caught a synchronous invocation (at least 100 ms) and again with a final `Expect(markerPath).NotTo(BeAnExistingFile())` after the context is cancelled.

   The point is the pair: the request reached the registry **and** the poster did not run. Either assertion alone is satisfiable by a broken implementation.

2. **Negative evidence: the empty `Source` behaves identically.** The same spec shape with `Source` left as the zero value — the compatibility path for a caller outside the interactive service, or a test driving the session directly. Assert the registry reports it pending and the marker file is absent.

3. **The pod's `GET /permission` route still serves a `/prompt` gate.** Add a spec to the existing `Describe("application.newInteractiveService")` block in `main_internal_test.go`:
   - build `app := &application{Listen: "127.0.0.1:0", ProviderBaseURL: "http://provider.example.test", InteractiveAuthToken: "test-token", A2APublicURL: "https://agent.example.test/a2a", A2AAgentName: "claude-interactive", AttentionStoreURL: "https://store.example.test"}` (mirror the Agent Card spec's values; ⚠️ `AttentionStoreURL` is set **only** so the "body carries no store URL" assertion below has teeth — with the field left empty that assertion can never fail);
   - `registry := interactive.NewPermissionRegistry()`;
   - `service := app.newInteractiveService(&agentmocks.SessionFactory{}, prometheus.NewRegistry(), registry)`;
   - start `registry.DecidePermission(ctx, claudelib.PermissionRequest{ToolName: "Bash", Description: "run a command", Source: claudelib.PromptPermissionSource})` in a goroutine, with a `context.WithCancel` cancelled via `DeferCleanup`;
   - `Eventually(registry.List()).Should(HaveLen(1))`;
   - `GET /permission` via `httptest.NewRequest` + `service.Handler().ServeHTTP(recorder, request)` with `request.Header.Set("Authorization", "Bearer test-token")`;
   - assert `recorder.Code` is `http.StatusOK`, that the body decodes to a JSON array containing the pending id, and that it carries `tool_name` equal to `"Bash"`;
   - assert the body does **not** contain the attention store's URL or any store host — the route answers from the registry, not the store.
   - Add the `claudelib "github.com/bborbe/agent/claude"` import if it is not already present; keep the existing Agent Card spec and the `Describe("application.runService")` block passing untouched.

4. **Record the store contract — new file `docs/attention-store.md`.** Nothing under `docs/`, `README.md` or `CLAUDE.md` currently describes the attention store, and this knowledge must outlive the spec. Derive the store's behaviour from `scripts/pod-attention.py` (read its module docstring, `verdict`, `cmd_gate`, `cmd_ask`, `render_poll`, `auth_refusal`) and `scripts/answered-attribution.py`; do not restate behaviour you have not read. ⚠️ **Two of the facts below are in NEITHER script — take them from this spec's own `## Constraints`, which states both, and say so in your report. Never invent them, and never omit them.** They are the store's **read surfaces** (`GET /api/1.0/attention`, open-only, versus `/api/1.0/attention/history`, all states — the store's own Go implementation is not in this repository, so neither script can show you the route) and the **`dedup_key` keying**. The verification step below greps for the `/history` string, so omitting it fails this prompt's own check. Cover:
   - what the store is for, and that the pod reaches it through `pod-attention.py` rather than through Go code;
   - the **classes** and how `answer_mechanism` distinguishes them, with `permission` as the class a gate uses;
   - the **`permission` release rule, verbatim**: `state == "answered" AND decision in {allow, deny} AND resolved_by is a non-empty string` — and therefore that **the board's JavaScript cannot release a gate**, because it sends neither `decision` nor `resolved_by`; only an arm answer can. ⚠️ State this as the **`permission` class's** rule, never as a store-wide one: it lives in `pod-attention.py`'s `verdict()`, and `answered-attribution.py` records that the same `resolved_by` requirement was **withdrawn on 2026-09-27** for that script's own (non-permission) predicate — the two files do not contradict each other, they govern different classes;
   - **`dedup_key` keying**: items are keyed by it, so two gates must never share one, and a second gate on the same producer gets its own item and its own answer;
   - the **three poll terminals** — `OPEN`, `ANSWERED:`, `NOT_OPERATOR_ANSWERED:` — and that a gate caller must read the third exactly as it reads `OPEN`, because the gate is not released. Note that `ANSWERED:` is not proof the operator answered, since a scripted browser reports `navigator.webdriver === false`;
   - **`/api/1.0/attention/history` (all states) versus the open-only list** — why a count taken from the open-only list is not evidence of "nothing was posted", which is the distinction the spec's AC5 depends on;
   - **auth**: `POD_ATTENTION_STORE_URL` / `ATTENTION_STORE_URL` and `POD_ATTENTION_TOKEN` are read from the environment at runtime and never baked into an image or manifest; a non-loopback store with no token is refused rather than attempted, and the token must never appear in a log line or a command line;
   - **fail-open, never fail-allow**: any error, malformed response or deadline prints `NO_DECISION:` and exits non-zero *without* a verdict, so the gate stays for the caller's own fallback.

   Write it as prose with short headed sections, in the register of the existing `docs/` files. Link it from `README.md` with one sentence near the Service mode section (e.g. "The attention store's contract — classes, the `permission` release rule and its read surfaces — is documented in `docs/attention-store.md`.").

5. **`CHANGELOG.md`.** Append to the `## Unreleased` section **prompt 1 adds** (⚠️ it does not exist before prompt 1 runs — if you are running this prompt alone, stop and report that rather than creating it) a `docs:` bullet naming `docs/attention-store.md` and what it records. Follow `changelog-guide.md`.

6. **Coverage.** The specs added here must leave `pkg/permission` at or above 80% statement coverage — `go test -coverprofile=/tmp/cover.out ./pkg/permission/... && go tool cover -func=/tmp/cover.out`. These specs are assertion-only and should not move the number down.
</requirements>

<constraints>
- **The interactive path's decider behaviour must not change.** `interactive.NewPermissionRegistry()` keeps serving every non-A2A request, and a `/prompt`-raised request must produce **no** store traffic. If a spec here fails, the fix belongs in `pkg/permission/decider.go` from prompt 1 — not in the test's expectations.
- **Do not weaken prompt 1's specs to make these pass.** Both sets must hold together.
- **Do not modify `scripts/pod-attention.py` or `scripts/answered-attribution.py`.** They are the contract's source, already shipped in the image; this prompt reads them and documents them.
- **Do not reimplement the store protocol in Go** and do not add a store client. The gate invokes the poster; nothing here talks to the store directly.
- **No credential in the image, the manifest or a log line.** Do not print `POD_ATTENTION_TOKEN` in a spec, a doc example or a log line; `main.go`'s existing `display:"length"` treatment is the precedent.
- **Never log or assert on `InputPreview` as published content.** The store payload names the tool, not the tool's input.
- **No new dependency.**
- Errors use `github.com/bborbe/errors` — never `fmt.Errorf`, never a bare `return err`, never `context.Background()` in non-test code.
- Tests use Ginkgo v2 / Gomega in external `*_test` packages (the `main_internal_test.go` specs are the documented exception — that file is `package main` because `application` is unexported).
- Do NOT commit. dark-factory handles git.
- Do NOT run `make buca`.
</constraints>

<verification>
```bash
make precommit
make test

ls docs/attention-store.md                                             # exists
grep -c 'resolved_by' docs/attention-store.md                          # >= 1
grep -n 'dedup_key' docs/attention-store.md                            # >= 1
grep -n 'answer_mechanism' docs/attention-store.md                     # >= 1
grep -n 'NOT_OPERATOR_ANSWERED' docs/attention-store.md                # >= 1
grep -n '/api/1.0/attention/history' docs/attention-store.md           # >= 1
grep -n 'attention-store.md' README.md                                 # one match

grep -n 'PromptPermissionSource' pkg/permission/decider_test.go        # >= 1 match
grep -n 'Authorization' main_internal_test.go                          # >= 1 match (the /permission route is gated)
grep -n '"/permission"' main_internal_test.go                          # >= 1 match

go test ./pkg/permission/... -ginkgo.focus='prompt'
go test ./pkg/permission/... -ginkgo.focus='permission'
go test . -ginkgo.focus='permission'
go test ./... -ginkgo.focus='permission'
go test -coverprofile=/tmp/cover.out ./pkg/permission/... && go tool cover -func=/tmp/cover.out | tail -1

awk '/^## /{sec=$0} /attention-store/{print "sits under: " sec}' CHANGELOG.md   # at least one line; all: ## Unreleased
```

`make precommit` must exit 0.

**This prompt cannot produce the spec's AC5 evidence.** AC5 is Post-Deploy (Rung-2): it needs a live interactive pod, a real `GET /permission` listing, a real `POST /permission` verdict resuming a turn, and a store item count read from `/api/1.0/attention/history` on the real store. The commands above produce the in-container half — the routing and no-store-traffic proof — and nothing more. Do not claim AC5 is verified; say plainly in your report which half you proved.
</verification>
