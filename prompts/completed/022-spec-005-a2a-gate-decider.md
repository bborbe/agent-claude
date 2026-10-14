---
status: completed
spec: [005-an-a2a-permission-gate-reaches-the-attention-store]
summary: 'Bumped bborbe/agent to v0.101.0 and added pkg/permission: a poster-backed Gate that publishes an A2A-raised permission request to the attention store and a Decider that routes on PermissionRequest.Source, wired into main.go''s session factory while the /prompt path keeps resolving through the shared PermissionRegistry.'
execution_id: agent-claude-permgate-exec-022-spec-005-a2a-gate-decider
dark-factory-version: v0.198.7
created: "2026-10-14T19:44:38Z"
queued: "2026-10-14T20:22:06Z"
started: "2026-10-14T20:22:07Z"
completed: "2026-10-14T20:29:25Z"
branch: dark-factory/an-a2a-permission-gate-reaches-the-attention-store
---

<summary>
- A permission gate raised by an A2A turn is now published to the operator's attention board instead of waiting silently on the pod's own permission route.
- The A2A turn blocks until the operator answers the gate on the board, and that answer releases it: allow runs the tool, deny refuses it.
- A gate nobody answers still denies when its own 15-minute deadline passes, so an unanswered gate never becomes an approval.
- If the attention store is unreachable, or the pod's token is missing or rejected, the gate denies and the pod log names the failure.
- A gate raised by the interactive `/prompt` route is untouched: it still appears on the pod's own permission route and is still answered there.
- A caller that names no route at all keeps today's behaviour exactly.
- The shared agent library is bumped to the release that lets a decider tell the two routes apart.
- Two gates open at once each get their own board item and their own answer; neither inherits the other's.
- The tool's input is never published — the board item names the tool only.
- The README describes the new behaviour for A2A turns and the CHANGELOG records it.
</summary>

<objective>
Bump `github.com/bborbe/agent` to v0.101.0 and add a permission decider that routes an A2A-raised permission request to the attention store — publishing it as a `permission`-class item through the pod's own shipped poster and blocking for the operator's verdict — while every other request keeps resolving through the existing permission registry.
</objective>

<context>
Read `docs/dod.md` — the Definition of Done you are graded against. Read the coding guides at `/home/node/.claude/plugins/marketplaces/coding/docs/`: `go-patterns.md`, `go-error-wrapping-guide.md`, `go-testing-guide.md`, `go-composition.md`, `go-factory-pattern.md`, `go-logging-guide.md`, `go-concurrency-patterns.md`, `go-security-linting.md`, `go-precommit.md`, `changelog-guide.md`.

### The problem this prompt closes

`bborbe/agent-claude` runs the A2A worker a manager spawns per task. When a turn calls a tool outside `--allowedTools`, the CLI raises a permission request and the pod's decider blocks on the pod's **own** `/permission` route. Measured live on 2026-10-14: the request appeared on `GET /permission`, the turn blocked 3 m 21 s, and a `POST /permission` verdict resumed it — but nothing about that gate reached the attention board, which is where the operator actually looks. In the same turn the same pod successfully posted a message-class item to the store, so the poster, the store and the credentials are all demonstrably working; only the decider does not use them.

### Verified library facts (read from the module cache; do not re-derive, but you may confirm)

`github.com/bborbe/agent@v0.100.0` — the version `go.mod:10` currently pins — has **zero** occurrences of `Source` in its `interactive` package. `v0.101.0` is the release that carries it. Do **not** assume it is already in the local module cache — it may not be; `go mod tidy` in requirement 1 fetches it (the tag is published upstream), after which `$(go env GOPATH)/pkg/mod/github.com/bborbe/agent@v0.101.0` exists.

`claude/claude-session.go` in v0.101.0:

```go
type PermissionRequest struct {
	// ToolName is the name of the tool the CLI wants to invoke.
	ToolName string
	// Description is the CLI's human-readable description of the invocation.
	Description string
	// InputPreview is a preview of the tool input. It is not logged.
	InputPreview string
	// Source identifies the interactive service route that raised the request — /prompt
	// or /a2a. It is the empty PermissionSource when the caller that built the session
	// named no route. It is read from the turn's context, never from the request's own
	// data.
	Source PermissionSource
}

type PermissionDecision struct {
	// Allow reports whether the tool invocation may proceed.
	Allow bool
	// Message is an optional explanation returned to the CLI on denial.
	Message string
}

type PermissionDecider interface {
	DecidePermission(ctx context.Context, request PermissionRequest) (PermissionDecision, error)
}
```

`claude/permission-source.go` in v0.101.0:

```go
type PermissionSource string

const (
	// PromptPermissionSource marks a request raised by a POST /prompt turn.
	PromptPermissionSource PermissionSource = "/prompt"
	// A2APermissionSource marks a request raised by a POST /a2a turn.
	A2APermissionSource PermissionSource = "/a2a"
)
```

The library sets `Source` at the call site (`claude/claude-session.go`): `Source: PermissionSourceFromContext(ctx)`, and `interactive/prompt.go` / `interactive/a2a-handler.go` stamp the context with `claude.PromptPermissionSource` / `claude.A2APermissionSource` respectively. **`PermissionRequest` carries no session id** — `ToolName`, `Description`, `InputPreview` and `Source` are the whole struct.

### The poster — invoke it, do not reimplement it

`scripts/pod-attention.py` is already copied into the image at `/usr/local/bin/pod-attention.py` by the `Dockerfile` (which also `chmod 0755`s it). It is the pod-side half of the operator channel and it already encodes the `permission` class, the `dedup_key` keying, the class-dependent release rule and the provenance rule. **Reimplementing the store call in Go is explicitly out of scope** — a second implementation would be a second thing to keep in step.

Its `gate` subcommand is the primitive this prompt uses. Verified from the file:

```
python3 pod-attention.py gate --dedup-key KEY --payload "..." [--producer-id ID] [--liveness-ref R] [--timeout 900] [--interval 2]
```

- `DEFAULT_GATE_TIMEOUT = 900.0` (15 minutes), `DEFAULT_GATE_INTERVAL = 2.0` (2 seconds).
- It POSTs a `permission`-class card, prints `ITEM_ID: <id>`, then polls its own item every `--interval` until a verdict or the deadline.
- On a verdict it prints `DECISION: <allow|deny> -- <reason>` and **exits 0**.
- On the deadline it prints `NO_DECISION: <reason>` and **exits 1**. It never invents a verdict: "a default of allow would release a gate nobody approved; a default of deny would refuse one nobody refused."
- It prints `REFUSED: ...` and exits 2 when the store is non-loopback and `POD_ATTENTION_TOKEN` is unset, or when no producer id can be resolved; `FAILED: ...` and exits 1 on an HTTP error from the store.
- It reads `POD_ATTENTION_STORE_URL` / `ATTENTION_STORE_URL` and `POD_ATTENTION_TOKEN` from its **own environment**.
- `_producer_or_refuse` takes `--producer-id`, falling back to `CLAUDE_CODE_SESSION_ID`, and **refuses** (exit 2) when neither is set.

### Repo facts

- `main.go:349` — `permissions := interactive.NewPermissionRegistry()`; the same instance is passed to `factory.CreateClaudeSessionFactory(...)` (the sessions' decider) and to `newInteractiveService(...)` (which serves the pod's `/permission` route). That sharing is deliberate and must be preserved.
- `main.go:94-97` — the `application` struct already reads `AttentionStoreURL string \`... env:"POD_ATTENTION_STORE_URL" ...\`` and `AttentionToken string \`... env:"POD_ATTENTION_TOKEN" ... display:"length"\``. They are **pod-level** environment variables, so the parent process has them; `buildClaudeEnv` copies them into the Claude child.
- `pkg/factory/factory.go:68` — `CreateClaudeSessionFactory(claudeConfigDir, agentDir, allowedTools, model, env, permissions claudelib.PermissionDecider) agentlib.SessionFactory`. Its **only** production call site is `main.go:360` (service mode). `cmd/run-task/main.go` builds a one-shot runner with no sessions and no decider — it is out of scope for this prompt and must keep compiling untouched.
- `pkg/factory/factory_test.go:137` calls `factory.CreateClaudeSessionFactory("", "", nil, "", nil, nil)` with a nil decider; a nil decider stays valid and that spec must keep passing.
- `main_internal_test.go:336` has a `Describe("application.runService")` block whose first spec starts the real service on `127.0.0.1:0` and cancels it. That spec exercises this prompt's wiring and must keep passing.
- `Makefile.precommit:65` — `generate:` runs `rm -rf mocks`, re-creates `mocks/mocks.go`, then `go generate -mod=mod ./...`. `tools.env` pins `COUNTERFEITER_VERSION ?= v6.12.2` and states the directive must be `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@<ver>`. `github.com/bborbe/agent@v0.101.0/claude/claude_suite_test.go:16` is the upstream precedent for the exact `-generate` line.
- `github.com/bborbe/agent/mocks` (importable as `agentmocks "github.com/bborbe/agent/mocks"`) already ships `ClaudePermissionDecider` and `InteractivePermissionRegistry` counterfeiter fakes. Use them in tests rather than hand-writing fakes.
</context>

<requirements>
1. **Bump the library.** In `go.mod`, move `github.com/bborbe/agent` from `v0.100.0` to `v0.101.0`; run `go mod tidy`. Leave the pre-existing `exclude` block untouched and add no `replace` directive. Confirm `claude.PermissionRequest.Source`, `claude.A2APermissionSource` and `claude.PromptPermissionSource` resolve before writing anything that uses them.

2. **New package `pkg/permission`.** Two files, `gate.go` and `decider.go`, both with the repo's BSD license header (`// Copyright (c) 2026 Benjamin Borbe All rights reserved.` / `// Use of this source code is governed by a BSD-style` / `// license that can be found in the LICENSE file.`), matching every other file in the repo.

3. **`pkg/permission/gate.go` — the poster-backed gate.**

   Declare the install path the image uses:
   ```go
   // PosterPath is where the image installs the pod-side attention poster. The
   // Dockerfile copies scripts/pod-attention.py here; the decider invokes it rather
   // than reimplementing the store protocol.
   const PosterPath = "/usr/local/bin/pod-attention.py"
   ```

   Declare the interface and its counterfeiter directive. The directive's `-o` path is resolved relative to the package directory, so `../../mocks/` lands the fake in the repo's `mocks/` directory:
   ```go
   //counterfeiter:generate -o ../../mocks/permission-gate.go --fake-name PermissionGate . Gate

   // Gate decides one permission request by publishing it to the attention store and
   // blocking until the operator answers it there.
   type Gate interface {
       Decide(ctx context.Context, request claudelib.PermissionRequest) claudelib.PermissionDecision
   }
   ```
   `Decide` returns no error on purpose: every poster outcome is a verdict or a denial, and a denial is the fail-closed answer the caller needs. The constructor returns the interface:
   ```go
   // NewGate returns a Gate that publishes each request through the poster at
   // posterPath and blocks for the operator's answer. producerID is the producer the
   // store attributes the item to.
   func NewGate(posterPath string, producerID string) Gate
   ```

   The private struct holds `posterPath string`, `producerID string`, a `counter atomic.Uint64`, and a `nonce int64` set in the constructor to `time.Now().UnixNano()`. The nonce is a **process-uniqueness token, not a business timestamp** — `libtime.CurrentDateTimeGetter` injection does not apply to it, and the comment on the field must say so. Comment the struct's purpose; the struct is private and named after the interface.

   `Decide` builds a dedup key that is unique per gate:
   ```go
   key := fmt.Sprintf("agent-claude-a2a-gate-%d-%d", g.nonce, g.counter.Add(1))
   ```
   Two gates raised in one process therefore never share a `dedup_key`, and a restarted process never reuses a previous process's keys — the store keys items by `dedup_key`, so a shared key would hand the new gate the old, already-answered item.

   `Decide` then runs the poster with `exec.CommandContext(ctx, g.posterPath, "gate", "--dedup-key", key, "--payload", payload, "--producer-id", g.producerID)`. Do **not** set `cmd.Env`: the poster reads `POD_ATTENTION_STORE_URL` / `POD_ATTENTION_TOKEN` from the inherited environment at decision time, which is exactly what "read from the pod's own environment, never baked into the image or a manifest" requires. Capture stdout and stderr into `bytes.Buffer`s. Add a `// #nosec G204` comment on the `exec.CommandContext` line with the reason that the path is a compile-time constant and the arguments are not shell-interpreted.

   Mapping, which is the whole contract:
   - `cmd.Run()` returns an error → `glog.Warningf` naming the dedup key, the error and the trimmed stderr, and return `claudelib.PermissionDecision{Allow: false, Message: "attention gate did not return a verdict"}`.
   - `cmd.Run()` succeeds **and** stdout carries a parsable `DECISION:` line → return that verdict: `allow` → `{Allow: true}`; `deny` → `{Allow: false, Message: <the reason after " -- ">}`.
   - Any other success — no `DECISION:` line, or a decision token that is neither `allow` nor `deny` — → `glog.Warningf` the trimmed stdout and return the same denial as above.
   - The exit code alone is **never** a verdict: a zero exit with no parsable decision is a denial.

   Parsing lives in an unexported `parseDecision(stdout string) (claudelib.PermissionDecision, bool)`: split stdout on `"\n"`, `strings.CutPrefix` each trimmed line with `"DECISION: "`, `strings.Cut` the remainder on `" -- "`, and accept only `allow` / `deny` as the token. Anything else returns `false`.

   Payload construction lives in an unexported `payloadFor(request claudelib.PermissionRequest) string`. It names the tool, and the CLI's description when there is one — `Permission requested for tool "<ToolName>"` or `Permission requested for tool "<ToolName>": <Description>`. It **must not** include `request.InputPreview` under any circumstance, and a comment must say why: the item is readable by anything holding the store token and the tool's input is not ours to publish.

4. **`pkg/permission/decider.go` — the router.**

   ```go
   // NewDecider returns a PermissionDecider that sends a request to the channel that
   // raised it: a request raised by an A2A turn goes to the gate, and every other
   // request — one raised by a /prompt turn, or one that named no route at all — goes
   // to registry, which is the decider the pod's own /permission endpoint serves.
   func NewDecider(registry claudelib.PermissionDecider, gate Gate) claudelib.PermissionDecider
   ```

   The private struct holds `registry claudelib.PermissionDecider` and `gate Gate`. `DecidePermission` branches on `request.Source`: when it is **not** `claudelib.A2APermissionSource` it returns `d.registry.DecidePermission(ctx, request)` unchanged, error and all; when it is, it returns `d.gate.Decide(ctx, request), nil`. The empty `Source` therefore falls into the registry branch, which is the compatibility path — a caller outside the interactive service, or a test driving the session directly, is unaffected.

5. **Wire it in `main.go`.** Keep `permissions := interactive.NewPermissionRegistry()` exactly as it is — the pod's `/permission` route must keep serving that registry, and the sessions and the endpoint must keep sharing one instance. Between that line and the `sessions := heartbeat.NewObservingSessionFactory(...)` call, construct the routing decider and pass it to the session factory **in place of** `permissions`:
   ```go
   decider := permission.NewDecider(
       permissions,
       permission.NewGate(permission.PosterPath, gateProducerID(a)),
   )
   ```
   `factory.CreateClaudeSessionFactory(...)` keeps its signature and its `claudelib.PermissionDecider` parameter — only the value handed to it changes. `newInteractiveService(sessions, registry, permissions)` keeps receiving the bare `permissions`. Import `"github.com/bborbe/agent-claude/pkg/permission"` — no alias is needed, because no `permission` identifier is in scope in `main.go` (`interactive` is already imported).

   Add the producer-id helper to `main.go`, next to `newInteractiveService`:
   ```go
   // gateProducerID is the producer a permission gate is posted to the attention
   // store under: the pod's own name, so the board attributes a gate to the worker
   // that raised it and two pods never share one producer. A pod always has a
   // hostname; the agent name is the fallback for the case where it does not, and the
   // service-mode guard has already established that the agent name is non-empty.
   func gateProducerID(a *application) string
   ```
   It calls `os.Hostname()`; on error it `glog.Warningf`s the failure and returns `a.A2AAgentName`. Add `"os"` to `main.go`'s imports if it is not already there.

6. **Counterfeiter plumbing.** Create `pkg/permission/permission_suite_test.go` in the `permission_test` package, matching `pkg/factory/factory_suite_test.go` exactly (same license header, `TestSuite(t *testing.T)` with `time.Local = time.UTC`, `format.TruncatedDiff = false`, `RegisterFailHandler(Fail)`, `suiteConfig.Timeout = 60 * time.Second`, `RunSpecs(t, "Permission Suite", suiteConfig, reporterConfig)`), and add this line above `func TestSuite`:
   ```go
   //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
   ```
   Run `make generate` and confirm `mocks/permission-gate.go` exists and declares `type PermissionGate struct`. If counterfeiter resolves the directive's `-o` against the module root rather than the package directory and the file lands somewhere else, correct the `-o` path so the fake ends up in `mocks/`, and say so in your report.

7. **Tests — `pkg/permission/decider_test.go`** in the external `permission_test` package, Ginkgo v2 / Gomega, using `agentmocks "github.com/bborbe/agent/mocks"` for `ClaudePermissionDecider` and the generated `mocks "github.com/bborbe/agent-claude/mocks"` for `PermissionGate`. Cover at minimum:
   - a `Source: claudelib.PromptPermissionSource` request returns the registry's decision and the gate's `DecideCallCount()` is **0**;
   - a request with the empty `Source` behaves identically — registry decides, gate never called (the compatibility path);
   - a `Source: claudelib.A2APermissionSource` request returns the gate's decision and the registry's `DecidePermissionCallCount()` is **0**;
   - the request the gate receives is the request the decider was handed, `Source` included — assert via the fake's `DecideArgsForCall(0)`;
   - a non-nil error from the registry on a `/prompt` request is returned unwrapped and unchanged.

8. **Tests — `pkg/permission/gate_test.go`** in the external `permission_test` package. Drive the **real** `NewGate` against a **fake poster**: write a small `#!/bin/sh` script into `GinkgoT().TempDir()`, `chmod 0700` it, and point `NewGate` at it. This is the subprocess boundary and the point of the file — shape assertions do not satisfy it. Cover at minimum:
   - a shim printing `ITEM_ID: 1` then `DECISION: allow -- operator said yes` and exiting 0 → `Allow` is true;
   - a shim printing `DECISION: deny -- operator said no` and exiting 0 → `Allow` is false and `Message` carries the reason;
   - a shim printing `NO_DECISION: store unreachable` and exiting 1 → `Allow` is false;
   - a shim printing `DECISION: allow -- x` and exiting **1** → `Allow` is false (the exit code alone is not a verdict);
   - a shim printing nothing parsable and exiting 0 → `Allow` is false;
   - a shim printing `REFUSED: ...` and exiting 2 → `Allow` is false;
   - `NewGate("/nonexistent/poster", "p")` → `Allow` is false and `Decide` does not panic;
   - the shim echoes `"$@"` to a file in the temp dir; assert the recorded argv carries `gate`, `--producer-id` with the producer the gate was constructed with, and a `--payload` that contains the tool name and does **not** contain the request's `InputPreview` (the security rule — publish the tool, never the input);
   - two consecutive `Decide` calls on one gate produce **different** `--dedup-key` values (each gate gets its own item).

9. **Coverage.** `go test -coverprofile=/tmp/cover.out ./pkg/permission/... && go tool cover -func=/tmp/cover.out` must report at least 80% for the package. The only line expected to be unreachable is the `os.Hostname()` error branch in `main.go`.

10. **`README.md`.** In the Service mode section, next to the existing sentence that the service serves "the permission endpoint", state what that means now: a permission gate raised by an A2A turn is published to the attention store as a `permission`-class item and answered there by the operator, while a gate raised by a `/prompt` turn still appears on the pod's own `GET /permission` route and is answered by a `POST /permission` verdict. Do not rewrite the surrounding paragraph — add to it.

11. **`CHANGELOG.md`.** Add `## Unreleased` above the newest existing heading with one `feat:` bullet: a permission gate raised by an A2A turn is published to the attention store as a `permission`-class item and blocks until the operator answers it there, while a `/prompt` turn's gate keeps resolving through the pod's own `/permission` route; `github.com/bborbe/agent` is bumped to v0.101.0 for `PermissionRequest.Source`. Follow `changelog-guide.md` — one bullet, prefix required, name the types.
</requirements>

<constraints>
- **The interactive path's decider behaviour must not change.** `interactive.NewPermissionRegistry()` keeps serving every non-A2A request; the new behaviour is added for one `Source` value only. A `/prompt`-raised request must produce **no** store traffic.
- **The poster is already in the image** (`Dockerfile` copies `pod-attention.py` and `answered-attribution.py` to `/usr/local/bin/`). Do not re-vendor, re-point, duplicate or modify it, and do not reimplement the store protocol in Go.
- **No credential in the image, the manifest or a log line.** The store token is read from the environment at decision time and must never be printed; `main.go`'s existing `display:"length"` treatment of `AttentionToken` is the precedent.
- **Never log `InputPreview`.** Do not start logging it in order to build a store payload — the payload names the tool, not the tool's input.
- **No new dependency** beyond what the store call needs. `bborbe/agent`'s `interactive` and `claude` packages are already imported; do not add `github.com/google/uuid` (it is an indirect dependency only) or any other module.
- **Fail-closed.** An unanswered gate, an unreachable store, a missing or rejected token, a malformed response and the 15-minute deadline all resolve to deny. Nothing on the A2A path may resolve to allow without a verdict the poster actually read.
- Errors use `github.com/bborbe/errors` — `errors.Wrap(ctx, err, "msg")`, `errors.New(ctx, "msg")`. Never `fmt.Errorf`, never a bare `return err`, never `context.Background()` in non-test code.
- Interface → Constructor → Struct → Method: the interface is exported, the implementation struct is private and named after it, `New*` returns the interface.
- Factory functions are pure composition — no conditionals, no I/O. (`gateProducerID` is a `main.go` helper, not a factory.)
- Tests use Ginkgo v2 / Gomega in external `*_test` packages, with counterfeiter mocks — never hand-written mocks.
- `cmd/run-task/main.go` is out of scope and must keep compiling unchanged. `pkg/factory/factory_test.go`'s nil-decider spec must keep passing.
- Do NOT commit. dark-factory handles git.
- Do NOT run `make buca` — the cluster pulls the mirrored quant-registry image, not docker.io.
</constraints>

<verification>
```bash
make precommit
make test

grep -n 'github.com/bborbe/agent v0.101.0' go.mod                        # one match
grep -c '^replace' go.mod                                                # prints 0
grep -n 'A2APermissionSource' pkg/permission/decider.go                  # >= 1 match
grep -n 'claudelib.A2APermissionSource' pkg/permission/decider.go        # >= 1 match
grep -n 'InputPreview' pkg/permission/gate.go                            # 0 matches (the input is never published)
grep -n 'POD_ATTENTION' pkg/permission/gate.go                           # 0 matches (the poster reads its own env)
grep -n 'cmd.Env' pkg/permission/gate.go                                 # 0 matches (inherit the environment)
grep -n 'permission.NewDecider' main.go                                  # one match
grep -n 'permission.NewGate' main.go                                     # one match
grep -n 'permission.PosterPath' main.go                                  # one match (the consumer qualifies it; inside package permission the field is written unqualified)
grep -n 'interactive.NewPermissionRegistry()' main.go                    # one match (still the registry the endpoint serves)
grep -n 'go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate' pkg/permission/permission_suite_test.go   # one match
ls mocks/permission-gate.go                                              # exists
grep -n 'type PermissionGate struct' mocks/permission-gate.go            # one match

go test -coverprofile=/tmp/cover.out ./pkg/permission/... && go tool cover -func=/tmp/cover.out | tail -1
go test ./pkg/permission/... -ginkgo.focus='permission'
go test ./... -ginkgo.focus='permission'

awk '/^## /{sec=$0} /v0.101.0/{print "sits under: " sec}' CHANGELOG.md   # at least one line; all: ## Unreleased
grep -c 'attention' README.md                                            # >= 1
```

`make precommit` must exit 0. It runs `generate`, so it regenerates `mocks/permission-gate.go` from scratch — a stale or hand-edited fake will not survive it.

**This prompt cannot produce the spec's AC1-AC5 evidence.** Those are Post-Deploy (Rung-2) and need a live pod turn against a live attention store; the commands above prove the decision mapping, the error taxonomy and the compatibility path only. Do not claim otherwise in your report.
</verification>
