---
status: draft
spec: [001-cluster-worker-heartbeat]
created: "2026-10-05T16:22:00Z"
branch: dark-factory/cluster-worker-heartbeat
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SETTLED — activity is recorded AFTER the delegate's Prompt returns, unconditionally on success
   or failure. Desired Behavior 4 measures the idle cutoff from the last *completed* prompt
   ("has not completed a prompt for 90 seconds"), so the timestamp must be the completion time, not
   the invocation time. Recording in Create instead would register only a session's first touch —
   the exact defect AC4 separates out.

2. SETTLED — the publisher's interval is a constructor argument (production passes
   heartbeat.RefreshInterval), not a package constant read inside Run. Without it the real loop
   cannot be exercised without a 20-second wait, and AC3 is about the loop. It is an internal
   mechanism parameter, NOT a config knob: no struct tag, env var or argument-struct field.

3. SETTLED — no counterfeiter annotations for ActivityRecorder/Writer/Publisher. This repository's
   `make precommit` regeneration step rebuilds only the root `mocks/` package from the root-package
   `//go:generate` directive in `main_test.go`; a directive under `pkg/heartbeat/` would not run and
   would risk breaking `make precommit`. The tests need no fake for these three interfaces — they
   use the real implementations plus the counterfeiter fakes `bborbe/agent` and `bborbe/k8s`
   already ship.

4. SETTLED — the writer treats an unreadable ConfigMap (Get returns an error) as "no existing data"
   and publishes a single-entry ConfigMap, letting Deploy's own internal Get/Create-or-Update path
   surface a genuine cluster failure. The alternative — inspecting the error for the Kubernetes
   NotFound type — was rejected because `k8s.ConfigMapDeployer.Get` wraps with
   `github.com/bborbe/errors`, whose chain cannot be assumed to preserve `apierrors.IsNotFound`, and
   the alternative is unverifiable from this container.

5. SETTLED — the pinned-clock idiom uses `libtime.DateTime(time.Date(...))` with `SetNow`.
   `github.com/bborbe/time` is a **direct** dependency (v1.27.14) already imported by `main.go` and
   `pkg/factory/factory_kafka_topic_test.go`, so its source IS in the module cache — `go doc
   github.com/bborbe/time` confirms the helper if the exact name differs. `go-time-injection.md` is
   the source for the pinned-clock idiom.
-->

# Publish cluster worker heartbeats: the writer, the activity observer and the refresh loop

<summary>
- A new package owns everything this change writes: nothing else in the repository learns about the ConfigMap.
- Each session the service actually serves becomes publishable, and its entry is re-stamped on a fixed cadence.
- The entry carries exactly the one field the fleet's liveness reader parses, in the format that reader expects.
- A session that stops being addressed stops being re-stamped, so its entry ages out on its own with nothing to clean up.
- The write merges into whatever the ConfigMap already holds, so a second writer's entries survive this writer's writes.
- A failing cluster write is reported to the caller rather than crashing the process, so prompt serving can continue.
- Nothing about the service's existing routes, authentication or response shapes is touched by this prompt.
- The reader's own source is not modified, and no credential is written into the entry.
- Unit tests cover the entry shape, the merge, the cadence, the idle cutoff, the observer and the failure path.
</summary>

<objective>
Create a new package `pkg/heartbeat` that provides the three units the cluster-heartbeat feature is built from: an activity recorder that remembers which sessions were recently served, a writer that stamps one session's liveness entry into the `claude-worker-heartbeats` ConfigMap in the shape the fleet's liveness reader consumes, and a session-factory decorator that feeds the recorder from the library's `Session.Prompt`. Plus a publisher that re-stamps every recently-served session on a fixed interval. Nothing is wired into the running service in this prompt — this prompt lands the units and their unit tests only.
</objective>

<context>
This repository has no root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md` (the Definition of Done you are graded against; it forbids `replace` directives in `go.mod`).

Files you will read before writing:
- `pkg/factory/factory.go` — the existing `Create*` composition layer; `CreateClaudeSessionFactory` is the factory whose returned `Session` this prompt's decorator wraps.
- `pkg/factory/factory_suite_test.go` — the Ginkgo v2 suite bootstrap you must copy for the new package.
- `pkg/factory/factory_test.go` — the existing external-test-package style (`package factory_test`).
- `pkg/factory/factory_kafka_topic_test.go` — how a `libtime.CurrentDateTimeGetter` is obtained (`currentDateTime = libtime.NewCurrentDateTime()`).
- `main.go` — read `runService` to understand where the pieces created here will later be consumed (this prompt does not change `main.go`).
- `go.mod` — `github.com/bborbe/k8s` is currently an **indirect** dependency (`github.com/bborbe/k8s v1.14.19`). Importing it makes it direct; `go mod tidy` (run by `make precommit`) promotes it. Do not hand-edit `go.mod`.

The library API you must build against — read the landed source in the module cache before you write anything. These signatures are quoted from the landed files and are the contract; do not paraphrase them.

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.94.0/agent_session.go
// package lib — import path github.com/bborbe/agent, already imported elsewhere as `agentlib`.
// The package clause is `package lib` but the import path is github.com/bborbe/agent.
type Session interface {
	Prompt(ctx context.Context, prompt string) (string, error)
	Close(ctx context.Context) error
}

type SessionFactory interface {
	Create(id string) Session
}
```

```go
// $(go env GOMODCACHE)/github.com/bborbe/k8s@v1.14.19/k8s_configmap-deployer.go
// package k8s — import path github.com/bborbe/k8s
type ConfigMapDeployer interface {
	Get(ctx context.Context, namespace Namespace, name Name) (*v1.ConfigMap, error)
	Deploy(ctx context.Context, configmap v1.ConfigMap) error
	Undeploy(ctx context.Context, namespace Namespace, name Name) error
}

func NewConfigMapDeployer(clientset k8s_kubernetes.Interface) ConfigMapDeployer
```

```go
// $(go env GOMODCACHE)/github.com/bborbe/k8s@v1.14.19/k8s_name.go, k8s_namespace.go
type Name string
type Namespace string
func (n Name) String() string
func (f Namespace) String() string
```

**Semantics of `Deploy` you must design against — this is the load-bearing fact of this prompt.** `ConfigMapDeployer.Deploy` is a **whole-object replace, not a `data`-map merge**. Its unexported `mergeConfigMap(current, updated)` copies only `current.ResourceVersion` onto `updated` and returns `updated`, so the `updated.Data` map **replaces** the stored `data` entirely. A writer that calls `Deploy` with a ConfigMap built from scratch for one key therefore silently wipes every other writer's keys. This writer must do a read-modify-write: `Get` the ConfigMap, merge its own key into the existing `.Data`, then `Deploy` the merged object. `Deploy` already creates the ConfigMap when its own internal `Get` fails, so the "ConfigMap does not exist yet" case is satisfied by calling `Deploy` — but you must not rely on that to protect other writers' keys.

**The reader's contract — the shape you must emit.** The reader is `scripts/cluster-heartbeat.py` in `bborbe/claude-supervisor`. It is NOT in this repository, it must NOT be modified, and it is the source of truth. Its contract, quoted from the spec (`scripts/cluster-heartbeat.py:54-65` and `:113-131`):
- the ConfigMap is named `claude-worker-heartbeats`;
- `data` maps `<session_id>` to a JSON string carrying a `refreshedAt` field in RFC3339 form;
- the liveness verdict is the stamp's age against `TTL_SECONDS = 60`;
- an absent ConfigMap is a readable-and-empty answer, not an error;
- an unparseable value is silently skipped, not an error.

Fixed numbers from the spec, and they are not configurable: **20 s refresh cadence**, **90 s idle cutoff**, **60 s reader TTL** (the last is the reader's; this package does not apply it).

The session id is written verbatim as a ConfigMap key. The library validates it at intake in `github.com/bborbe/agent/interactive/session-id.go` (pattern `^[A-Za-z0-9_][A-Za-z0-9_-]{0,63}$`, defaulting to `identity` when the header is absent), so this package **inherits a validated value** and must not re-validate or transform it. Do not add a second validator, and do not reject or rewrite the id.

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-logging-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Create the package with its constants.** New file `pkg/heartbeat/heartbeat.go` holding the package doc comment and the three fixed values. `ConfigMapName` is the reader's name and is not configurable here; the two durations are the spec's fixed numbers and are package constants, not fields, not arguments, not env vars.

   ```go
   // Package heartbeat publishes a liveness entry per actively-served session
   // into the claude-worker-heartbeats ConfigMap, in the shape the cluster
   // liveness reader (scripts/cluster-heartbeat.py in bborbe/claude-supervisor)
   // consumes. The reader is the source of truth and is not modified: it maps
   // each session id to a JSON object carrying a refreshedAt field in RFC3339
   // form and treats a stamp older than 60 seconds as dead.
   package heartbeat

   // ConfigMapName is the ConfigMap the reader reads and this package writes.
   const ConfigMapName k8s.Name = "claude-worker-heartbeats"

   // RefreshInterval is how often an active session's entry is re-stamped. It is
   // deliberately shorter than the reader's 60-second TTL — three refreshes fit
   // in one TTL window, so a single missed tick cannot make a live worker read as
   // stale.
   const RefreshInterval = 20 * time.Second

   // IdleCutoff is how long a session may go without completing a prompt before
   // its entry stops being refreshed. The entry then ages out of the reader's
   // view on its own, so an idle conversation does not read as a live worker.
   const IdleCutoff = 90 * time.Second
   ```

   `k8s.Name` is a string type, so a typed string constant is valid Go. Import `github.com/bborbe/k8s` and `time`.

2. **Add the activity recorder.** New file `pkg/heartbeat/activity.go`.

   ```go
   // ActivityRecorder remembers which sessions this service has recently served.
   type ActivityRecorder interface {
       // Record marks sessionID as active as of now. It is called when a prompt
       // on that session returns.
       Record(ctx context.Context, sessionID string)

       // Active returns the ids of every session that recorded activity within
       // cutoff of now, sorted so a caller's output is deterministic.
       Active(ctx context.Context, cutoff time.Duration) []string
   }

   // NewActivityRecorder returns an ActivityRecorder that timestamps activity
   // with the supplied clock. The clock is injected so a test can pin it.
   func NewActivityRecorder(currentDateTime libtime.CurrentDateTimeGetter) ActivityRecorder
   ```

   The unexported implementation holds `currentDateTime libtime.CurrentDateTimeGetter`, a `sync.Mutex`, and a `map[string]libtime.DateTime` of last activity per id. `Record` stores `currentDateTime.Now()` under the mutex. `Active` computes `now := currentDateTime.Now().Time()` once, then returns every id whose `now.Sub(last.Time()) < cutoff`, sorted (use `slices.Sort`) and preallocated with `make([]string, 0, len(...))` (`prealloc` is enabled). Do NOT delete aged-out entries: the map is small, and pruning is not required by the spec.

   Use `libtime.DateTime` for the stored timestamps and convert with `.Time()` for arithmetic — that conversion is the one `main.go` already relies on (`libtime.NewCurrentDateTime().Now().Time()`). Never call `time.Now()`; the clock is injected.

3. **Add the writer.** New file `pkg/heartbeat/writer.go`.

   ```go
   // Writer publishes the liveness entry for one session.
   type Writer interface {
       // Write stamps sessionID into the heartbeat ConfigMap with the current time.
       Write(ctx context.Context, sessionID string) error
   }

   // NewConfigMapWriter returns a Writer that stamps entries into the named
   // ConfigMap in the given namespace, reading the existing data first so the
   // write merges with what other writers have stored instead of replacing it.
   func NewConfigMapWriter(
       deployer k8s.ConfigMapDeployer,
       namespace k8s.Namespace,
       name k8s.Name,
       currentDateTime libtime.CurrentDateTimeGetter,
   ) Writer
   ```

   The unexported `configMapWriter` holds the four values. `Write` does exactly this, in this order:

   a. Marshal the entry value. The entry is a struct with **exactly one** field — the reader requires no other, and a field the reader cannot parse is one more way for the shape to drift:

   ```go
   // heartbeatEntry is the value the reader parses. Only refreshedAt is written:
   // the reader requires no other field.
   type heartbeatEntry struct {
       RefreshedAt string `json:"refreshedAt"`
   }
   ```

   Stamp it with `w.currentDateTime.Now().Time().UTC().Format(time.RFC3339)`. Use `encoding/json` to marshal; handle the error with `errors.Wrap(ctx, err, "marshal heartbeat entry")` from `github.com/bborbe/errors`.

   b. Read the existing ConfigMap: `current, err := w.deployer.Get(ctx, w.namespace, w.name)`.

   c. Build the merged `data` map:
   - if `err != nil`: the existing state is not readable. Log at `glog.V(3)` that the read failed, and publish a **single-entry** ConfigMap. Note in the code comment that this assumes the failure is **persistent**: `Deploy`'s internal `Get` re-reads and surfaces a persistent failure, but a **transient** read failure (timeout / 429 / 500) while the ConfigMap holds another writer's keys would let `Deploy`'s whole-object replace (`mergeConfigMap` carries only `ResourceVersion`) drop those keys for one tick — bounded and self-healing, because each writer re-stamps its own entry within `RefreshInterval`.
   - if `err == nil`: start from `current.Data` (create an empty map when it is nil), set `data[sessionID] = string(value)`, and deploy the merged map. This is the read-modify-write that stops this writer from erasing another writer's keys.

   d. Call `w.deployer.Deploy` with a `corev1.ConfigMap` carrying `metav1.ObjectMeta{Name: w.name.String(), Namespace: w.namespace.String()}` and the merged `Data`. Wrap a non-nil error with `errors.Wrap(ctx, err, "deploy heartbeat configmap")` and return it — the caller decides whether a failed write is fatal (it is not; see requirement 4). Do NOT swallow the error here.

   Imports: `corev1 "k8s.io/api/core/v1"` (required by the `Deploy` signature, which takes `v1.ConfigMap`) and `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` (required to set that object's `ObjectMeta`). Build the ConfigMap with a small unexported helper method on `configMapWriter` so both deploy sites share it.

   The entry carries a session id and a timestamp and **nothing else**. Do not add the bearer token, any environment value, a pod name or a task id to the value or the key.

4. **Add the publisher.** New file `pkg/heartbeat/publisher.go`.

   ```go
   // Publisher re-stamps the entry for every recently-active session on a fixed
   // interval until the context is cancelled.
   type Publisher interface {
       // Run refreshes active sessions every interval and returns nil when the
       // context is cancelled. A failing write is logged and skipped: a broken
       // liveness path must not take prompt serving down.
       Run(ctx context.Context) error
   }

   // NewPublisher returns a Publisher that re-stamps every session active within
   // IdleCutoff once per interval.
   func NewPublisher(recorder ActivityRecorder, writer Writer, interval time.Duration) Publisher
   ```

   The interval is a constructor argument (production passes `heartbeat.RefreshInterval`) so a test can drive the real loop without waiting 20 seconds. It is an internal mechanism parameter, not a config knob — do not add a struct tag, an env var or an argument-struct field for it anywhere.

   `Run` logs `glog.V(2)` that the publisher started (with the interval), builds `time.NewTicker(interval)` with `defer ticker.Stop()`, and loops:

   ```go
   for {
       select {
       case <-ctx.Done():
           return nil
       case <-ticker.C:
           p.refresh(ctx)
       }
   }
   ```

   `refresh` iterates `p.recorder.Active(ctx, IdleCutoff)` and calls `p.writer.Write(ctx, sessionID)` for each; on a write error it logs `glog.Warningf("cluster heartbeat write failed session=%s: %v", sessionID, err)` and **continues to the next session**. `Run` never returns a write error — the only exit is context cancellation, which returns nil. This is the whole of the "a failing cluster write does not take the service down" behavior: the service composes this `Run` with its own, so a returned error would tear the service down.

   The log line must contain the session id and the error and nothing else. Do not log the bearer token or any environment value — the publisher never has them, and must not be given them.

5. **Add the session-activity observer.** New file `pkg/heartbeat/observer.go`.

   ```go
   // NewObservingSessionFactory wraps delegate so that every session it builds
   // records its id with recorder when a prompt returns. The library calls
   // SessionFactory.Create only on first use and never again, so the activity
   // signal has to be captured on the Session it returns, not on Create.
   func NewObservingSessionFactory(
       delegate agentlib.SessionFactory,
       recorder ActivityRecorder,
   ) agentlib.SessionFactory
   ```

   The decorator's `Create(id string)` returns a wrapper around `delegate.Create(id)` that remembers the id. The wrapper's `Prompt(ctx, prompt)` calls `delegate.Prompt(ctx, prompt)`, then calls `recorder.Record(ctx, id)`, then returns the delegate's `(result, err)` unchanged — record **after** the delegate returns, because the spec's idle cutoff is measured from the last **completed** prompt. Record unconditionally (whether or not the prompt succeeded): a turn that failed still means the worker was addressed and is running. Put that reasoning in the code comment. `Close(ctx)` delegates unchanged.

   Do NOT record in `Create`: the library calls `Create` once per session, so recording there would register only a session's first touch and never its later prompts — which is exactly the defect the spec's fourth acceptance criterion separates out.

6. **Write the unit tests.** New package-internal suite bootstrap `pkg/heartbeat/heartbeat_suite_test.go`, `package heartbeat_test`, copied from `pkg/factory/factory_suite_test.go` (same `TestSuite`, `time.Local = time.UTC`, `format.TruncatedDiff = false`, 60 s suite timeout, `RunSpecs(t, "Heartbeat Suite", ...)`). All other test files are `package heartbeat_test` (external), so they exercise only the exported surface.

   In the test files, construct sentinel error values with the **standard library** `errors` package (`errors.New("...")`). The `github.com/bborbe/errors` convention in `<constraints>` applies to production code; a test double that needs a non-nil error value does not need a context, and importing the wrapping package into a `_test.go` file only creates an ambiguity with the standard package.

   For a deterministic clock, take `currentDateTime := libtime.NewCurrentDateTime()` (do NOT name the type — let it be inferred) and pin it with `currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))`. `libtime.DateTime` converts from `time.Time` and `SetNow` takes a `libtime.DateTime`; `go-time-injection.md` shows the canonical pinning idiom if the exact helper differs. Pass the same `currentDateTime` value into the recorder/writer under test, and call `SetNow` again later to advance it — the injected value is shared, so the code under test sees the new time.

   a. `pkg/heartbeat/activity_test.go` — `Describe("ActivityRecorder")`:
   - returns nothing before anything is recorded;
   - returns a recorded id while it is within the cutoff;
   - drops an id once the cutoff has elapsed since its last record (advance the pinned clock past `IdleCutoff`);
   - keeps an id that records again after the cutoff would have elapsed;
   - returns ids sorted;
   - pins the spec's numbers: `heartbeat.RefreshInterval == 20*time.Second`, `heartbeat.IdleCutoff == 90*time.Second`, and `heartbeat.RefreshInterval < 60*time.Second` (the reader's TTL — the margin the spec's third desired behavior is about).

   b. `pkg/heartbeat/writer_test.go` — `Describe("ConfigMapWriter")`, using the library's counterfeiter fake `K8sConfigMapDeployer` from `github.com/bborbe/k8s/mocks`, imported as `k8smocks` (both `github.com/bborbe/k8s` and its `mocks` package are imported, so the alias is required for readability). Its setters and accessors are `GetReturns(*v1.ConfigMap, error)`, `DeployCallCount() int`, `DeployArgsForCall(i int) (context.Context, v1.ConfigMap)`, `DeployReturns(error)`.
   - writes an entry keyed by the session id: after `Write(ctx, "abc")`, `DeployCallCount()` is 1, the deployed object's `Name`/`Namespace` are the configured ones, `deployed.Data` has key `"abc"`, and unmarshalling that value into `map[string]any` yields **exactly one** key, `refreshedAt`, whose string parses with `time.Parse(time.RFC3339, ...)`. The one-key assertion is the guard for the "no other field is added" behavior and for "no credential reaches this store".
   - merges into existing data: seed `GetReturns` with a `*corev1.ConfigMap` whose `Data` already holds another writer's key, call `Write(ctx, "abc")`, and assert the deployed `Data` contains **both** the pre-existing key and `"abc"`. This is the regression guard for the whole-object-replace semantics of `Deploy` — a writer that builds a fresh single-key ConfigMap fails here.
   - publishes a single-entry ConfigMap when the read finds nothing: `GetReturns(nil, errors.New("not found"))` → `Write` succeeds, `DeployCallCount()` is 1, and the deployed `Data` has exactly one key, `"abc"`.
   - returns the deploy error to the caller: `DeployReturns(errors.New("configmaps is forbidden"))` → `Write` returns a non-nil error whose message contains `forbidden`. This is the boundary the publisher's "log and continue" relies on.
   - calls `heartbeat.ConfigMapName.Validate(ctx)` and expects nil — the library-qualified `k8s.Name` boundary the constant crosses.

   c. `pkg/heartbeat/observer_test.go` — `Describe("ObservingSessionFactory")`, using the library's counterfeiter fakes `Session` and `SessionFactory` from `github.com/bborbe/agent/mocks`, imported as `agentmocks` (accessors `PromptReturns(string, error)` and `CreateReturns(lib.Session)`; `CreateReturns` takes the interface type from `github.com/bborbe/agent`, so a `*agentmocks.Session` satisfies it) and a **real** `heartbeat.NewActivityRecorder` as the recorder, asserting through `recorder.Active(ctx, heartbeat.IdleCutoff)`:
   - `Active` is empty before any prompt, and contains exactly `[]string{"abc"}` after `observing.Create("abc").Prompt(ctx, "hello")` returns;
   - the delegate's result is returned unchanged (set `PromptReturns("the answer", nil)`, expect `"the answer"`);
   - the id is recorded even when the prompt fails (`PromptReturns("", errors.New("boom"))` → the call returns an error and `Active` still contains `"abc"`);
   - `Close` delegates to the wrapped session.

   d. `pkg/heartbeat/publisher_test.go` — `Describe("Publisher")`, driving the real `NewPublisher` with a real recorder, a real `NewConfigMapWriter` and the `k8smocks.K8sConfigMapDeployer` fake, with a **short interval** (`10 * time.Millisecond`) so the real loop can be exercised. Every spec starts the loop the same way: a cancellable child of `ctx`, a `go func()` writing `publisher.Run(runCtx)` to a buffered channel, the spec's assertions, then `cancel()` and `Eventually(done).Should(Receive(BeNil()))`:
   - refreshes every active session on each tick and returns nil on cancel: record two ids, run `publisher.Run` in a `go func()` writing its result to a buffered channel (`go func()` is fine in `_test.go`), `Eventually(deployer.DeployCallCount).Should(BeNumerically(">=", 2))`, cancel the context, and `Eventually(done).Should(Receive(BeNil()))`;
   - writes nothing when no session has been served: empty recorder → start the loop → `Consistently(deployer.DeployCallCount, 100*time.Millisecond, 10*time.Millisecond).Should(Equal(0))` (ten intervals of no writes is what proves "several ticks, nothing written" without a fixed sleep);
   - does not refresh a session that has been idle past the cutoff: record one id, advance the pinned clock by 2 minutes (`SetNow`, past `heartbeat.IdleCutoff`), start the loop → `Consistently(deployer.DeployCallCount, 100*time.Millisecond, 10*time.Millisecond).Should(Equal(0))`;
   - keeps running when a write fails: `DeployReturns(errors.New("configmaps is forbidden"))`, record one id, run, `Eventually(deployer.DeployCallCount).Should(BeNumerically(">=", 2))` (proves it kept ticking after failures), then cancel and assert `Run` returned nil (proves the write error did not propagate).

   Give each spec's `BeforeEach` a fresh `context.Background()` and a fresh fake; never share a fake across specs.

7. **Do not add counterfeiter annotations or generated fakes for the new interfaces.** The tests above need no fake for `ActivityRecorder`, `Writer` or `Publisher` — they use the real implementations plus the counterfeiter fakes the `bborbe/agent` and `bborbe/k8s` modules already ship. This repository's `make precommit` regeneration step only rebuilds the root `mocks/` package from the root-package directive in `main_test.go`; a directive added under `pkg/heartbeat/` would not be picked up and would risk breaking `make precommit`. Leave `mocks/` and `main_test.go` untouched.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made, and confirm `go test -cover ./pkg/heartbeat/` reports ≥80% statement coverage. In particular confirm (i) the entry value carries exactly one field, (ii) the writer's merge path starts from the existing `Data`, not from an empty map, (iii) no `time.Now()` appears anywhere in `pkg/heartbeat/`, and (iv) `main.go` is unchanged by this prompt.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- The reader's own source is unmodified and is not in this repository: `scripts/cluster-heartbeat.py` lives in `bborbe/claude-supervisor`. Do not create it here, do not copy it here, and do not modify it. Its contract is quoted in `<context>`; satisfy it rather than bending it.
- Do NOT modify `github.com/bborbe/agent`. `Session` (two methods) and `SessionFactory` (one method) are frozen external contracts; wrap them, never change them, and do not add a method to either.
- Do NOT add a config knob for the refresh cadence or the idle cutoff. `RefreshInterval` and `IdleCutoff` are package constants with the spec's fixed values; the publisher's interval is a constructor argument only so a test can drive the loop. No new `application` struct field, no new env var, no new CLI flag in this prompt.
- Do NOT write the bearer token, any environment value, a pod name or a task id into the ConfigMap or into a log line. The entry is a session id and a timestamp and nothing else.
- Do NOT re-validate or transform the session id. The library validates it at intake (`github.com/bborbe/agent/interactive/session-id.go`, pattern `^[A-Za-z0-9_][A-Za-z0-9_-]{0,63}$`); this package inherits a validated value and writes it verbatim as the ConfigMap key.
- Do NOT add a `replace` directive to `go.mod` — `docs/dod.md` forbids it (it breaks remote install). Do not hand-edit `go.mod` at all: `go mod tidy` (run by `make precommit`) promotes `github.com/bborbe/k8s` and the `k8s.io/*` modules from indirect to direct.
- Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command. `vendor/` is a build-time artifact, gitignored, and wiped by `make precommit`.
- Do NOT touch `main.go`, `pkg/factory/`, `cmd/run-task/`, `mocks/` or `README.md` in this prompt. The wiring, the README note and the changelog entry belong to the next prompt in this batch. `cmd/run-task/main.go` is the local file-based CLI mode: it does not import `pkg/heartbeat`, so it must still compile untouched.
- Errors use `github.com/bborbe/errors` — `errors.Wrap(ctx, err, "msg")`, `errors.New(ctx, "msg")`, `errors.Errorf(ctx, "fmt", ...)`. Never `fmt.Errorf`, never a bare `return err`.
- Logging uses `github.com/golang/glog`: `V(2)` for the publisher's start, `V(3)` for per-item detail (the unreadable-ConfigMap case), `Warningf` for a failed write.
- Never call `time.Now()` in `pkg/heartbeat/` — inject `libtime.CurrentDateTimeGetter` into the constructors and call `Now()`.
- Exported types, functions and constants carry doc comments starting with their name. Interface → Constructor → Struct → Method. Constructors return interfaces.
- Tests use Ginkgo v2 / Gomega, in an external `package heartbeat_test`.
- Keep every file well under the repository's 2000-line `revive` limit and every function under the 80-line / 50-statement `funlen` limit.
</constraints>

<verification>
Run each of these and confirm the stated result. `make precommit` runs at the repository root; the `ROOTDIR=/workspace` prefix pins that root path explicitly, because this repository's `Makefile.variables` otherwise resolves `ROOTDIR` from a repository-root probe that is not reliable under the execution container.

1. `ROOTDIR=/workspace make test` — exits 0, and the new `pkg/heartbeat` specs are included in the run.
2. `ROOTDIR=/workspace make precommit` — exits 0.
3. `grep -rn 'refreshedAt' pkg/heartbeat/` — prints at least one line (the writer emits the field the reader parses).
4. `grep -c 'json:"refreshedAt"' pkg/heartbeat/writer.go` — prints `1`.
5. `grep -rn 'claude-worker-heartbeats' pkg/heartbeat/` — prints the constant in `heartbeat.go`.
6. `! grep -rnE '\btime\.Now\(\)' pkg/heartbeat/*.go` — exits 0 (the clock is injected everywhere; `\b` keeps this from matching `currentDateTime.Now()`).
7. `grep -c 'k8s.ConfigMapDeployer' pkg/heartbeat/writer.go` — prints a count greater than `0` (the writer depends on the deployer interface, not a concrete clientset).
8. `! grep -q 'heartbeat' main.go` — exits 0 (this prompt wires nothing into the running service; the wiring is the next prompt in this batch).
9. `go build ./...` — succeeds (the new package and its `k8s`/`k8s.io` imports compile).
</verification>
