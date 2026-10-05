---
status: draft
spec: [001-cluster-worker-heartbeat]
created: "2026-10-05T16:22:00Z"
branch: dark-factory/cluster-worker-heartbeat
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SETTLED — the reader's parser is mirrored in Go. The reader is
   `scripts/cluster-heartbeat.py` in `bborbe/claude-supervisor` — Python, in another repository,
   explicitly not modified. "Parses through the reader's own parser" therefore means a Go helper
   (`parseReaderEntry` + `readerLive`) that reproduces `json.loads` → `refreshedAt` →
   `datetime.fromisoformat` and the `age >= 0 && age < ttl` verdict. The prompt forbids shelling out
   to or importing the Python file.

2. SETTLED — the fake cluster surface is a hand-written, stateful, in-memory `k8s.ConfigMapDeployer`,
   not a counterfeiter mock. Counterfeiter fakes are stateless, and this test's whole point is the
   stored state after the loop runs (the merge, the shape). The prompt pins the one behaviour the
   fake must reproduce faithfully: `Deploy` replaces `Data` wholesale, matching the library's
   unexported `mergeConfigMap` (which carries only the ResourceVersion across). A fake that merged
   `Data` would let a writer that forgot the read-modify-write pass.

3. SETTLED — the `data` shape is asserted behaviourally (JSON parse → RFC3339 parse → the reader's
   age verdict), per the spec's Verification section ("a behavioral assertion, not a literal-count
   one"). The one-key assertion is included only to pin "no other field is added" / "no credential
   reaches this store".
-->

# Lock the heartbeat data shape against the reader's parser

<summary>
- An integration test drives the real refresh loop against an in-memory stand-in for the cluster's ConfigMap store.
- The test asserts the stored value round-trips through the reader's own parse — a JSON object with a `refreshedAt` string that parses as RFC3339 — rather than counting characters or fields.
- A drifting `data` shape fails a test here instead of silently disappearing from the reader's view on the cluster.
- The test also proves the write merges: a second writer's entry survives this writer's writes in the stored state.
- The test proves the idle cutoff stops the refresh, so an idle session's entry ages out.
- The test proves the loop keeps re-stamping, so the entry reads live under the reader's 60-second TTL after the loop has run.
- No production code changes: this prompt adds test files only.
- The reader's own source is not modified, and no credential is written into the entry.
</summary>

<objective>
Add an integration test to `pkg/heartbeat` that drives the real publisher, writer and activity recorder against an in-memory implementation of the cluster's ConfigMap store, and asserts the emitted `data` value parses through the reader's own contract (a JSON object carrying a `refreshedAt` string that parses as RFC3339, with the reader's age-versus-TTL verdict). This is the test that turns a drifting data shape into a red test rather than a silent cluster read, and it proves the merge and the idle cutoff end to end in stored state.
</objective>

<context>
This repository has no root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md`.

This prompt depends on the first prompt in this batch (`1-spec-001-heartbeat-writer.md`), which created `pkg/heartbeat` with `NewActivityRecorder`, `NewConfigMapWriter`, `NewPublisher`, the exported constants, and the Ginkgo suite bootstrap `pkg/heartbeat/heartbeat_suite_test.go`. If that package or its suite bootstrap does not exist, stop and report — do not create it yourself. It also depends on `pkg/heartbeat/writer_test.go` and `pkg/heartbeat/publisher_test.go` existing; read them so this prompt's specs complement them instead of duplicating them.

The exported surface you will drive (from the first prompt in this batch):

```go
const ConfigMapName k8s.Name = "claude-worker-heartbeats"
const RefreshInterval = 20 * time.Second
const IdleCutoff = 90 * time.Second

func NewActivityRecorder(currentDateTime libtime.CurrentDateTimeGetter) ActivityRecorder
func NewConfigMapWriter(deployer k8s.ConfigMapDeployer, namespace k8s.Namespace, name k8s.Name, currentDateTime libtime.CurrentDateTimeGetter) Writer
func NewPublisher(recorder ActivityRecorder, writer Writer, interval time.Duration) Publisher
```

`ActivityRecorder.Record(ctx, sessionID)` and `ActivityRecorder.Active(ctx, cutoff)` are the two methods; `Writer.Write(ctx, sessionID) error` is the writer's; `Publisher.Run(ctx) error` blocks until the context is cancelled and returns nil.

The interface you must implement as the fake cluster surface:

```go
// $(go env GOMODCACHE)/github.com/bborbe/k8s@v1.14.19/k8s_configmap-deployer.go
// package k8s — import path github.com/bborbe/k8s
type ConfigMapDeployer interface {
	Get(ctx context.Context, namespace Namespace, name Name) (*v1.ConfigMap, error)
	Deploy(ctx context.Context, configmap v1.ConfigMap) error
	Undeploy(ctx context.Context, namespace Namespace, name Name) error
}
```

**The `Deploy` semantics your fake must model.** The library's real `Deploy` is a **whole-object replace, not a `data`-map merge**: its unexported `mergeConfigMap(current, updated)` copies only `current.ResourceVersion` onto `updated` and returns `updated`, so the supplied `Data` map replaces the stored `data` entirely. Your fake must reproduce that — store the object it is handed, wholesale — because that is what makes the merge assertion meaningful: a writer that builds a fresh single-key ConfigMap loses every other writer's key here exactly as it would against a real cluster. A fake whose `Deploy` merged `Data` would let that bug pass.

**The reader's contract the test must mirror.** The reader is `scripts/cluster-heartbeat.py` in `bborbe/claude-supervisor`. It is NOT in this repository, it must NOT be modified, and it is the source of truth. Its contract, quoted from the spec (`scripts/cluster-heartbeat.py:54-65` and `:113-131`):
- the ConfigMap is named `claude-worker-heartbeats`;
- `data` maps `<session_id>` to a JSON string carrying a `refreshedAt` field in RFC3339 form;
- the liveness verdict is the stamp's age against `TTL_SECONDS = 60`;
- an absent ConfigMap is a readable-and-empty answer, not an error;
- an unparseable value is silently skipped, not an error.

Because the reader is Python and lives in another repository, the test reimplements its parse and its verdict in Go as two small helpers — that is what "parses through the reader's own parser" means here. Do not try to shell out to the Python file, and do not import it.

For a deterministic clock, take `currentDateTime := libtime.NewCurrentDateTime()` (do NOT name the type — let it be inferred) and pin it with `currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))`. `libtime.DateTime` converts from `time.Time` and `SetNow` takes a `libtime.DateTime`; `go-time-injection.md` shows the canonical pinning idiom if the exact helper differs. Pass the same `currentDateTime` value into the recorder and the writer, and call `SetNow` again to advance it — the injected value is shared, so the code under test sees the new time.

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-test-types-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Add the in-memory fake cluster surface.** New file `pkg/heartbeat/configmap_deployer_fake_test.go`, `package heartbeat_test`. It is a test helper, not production code, and it implements `k8s.ConfigMapDeployer`:

   ```go
   // fakeCluster is an in-memory stand-in for the cluster's ConfigMap store. Its
   // Deploy replaces the stored object wholesale — the same semantics as
   // k8s.NewConfigMapDeployer, whose mergeConfigMap only carries the ResourceVersion
   // across and lets the supplied Data replace the stored one. Modelling that
   // faithfully is what makes the merge test meaningful: a writer that builds a fresh
   // single-key ConfigMap loses every other writer's key here exactly as it would
   // against a real cluster.
   type fakeCluster struct {
       mutex       sync.Mutex
       items       map[string]corev1.ConfigMap
       deployCount int
   }

   func newFakeCluster() *fakeCluster

   // key namespaces the store by "<namespace>/<name>", the identity the real
   // deployer uses.
   func (f *fakeCluster) key(namespace k8s.Namespace, name k8s.Name) string

   func (f *fakeCluster) Get(ctx context.Context, namespace k8s.Namespace, name k8s.Name) (*corev1.ConfigMap, error)
   func (f *fakeCluster) Deploy(ctx context.Context, configmap corev1.ConfigMap) error
   func (f *fakeCluster) Undeploy(ctx context.Context, namespace k8s.Namespace, name k8s.Name) error

   // Two helpers the specs use to read the fake back:
   func (f *fakeCluster) stored(namespace k8s.Namespace, name k8s.Name) (corev1.ConfigMap, bool)
   func (f *fakeCluster) deployCalls() int
   ```

   Behaviour:
   - `Get` returns a **deep copy** of the stored object — clone the `Data` map as well as the struct, so a caller mutating the returned object cannot reach the stored map — and `true`-like success, or `nil` plus an error when nothing is stored (construct that error with the standard library `errors` package, e.g. `errors.New("configmap not found")` — the specs need a value that is simply non-nil). **The deep copy is load-bearing**: the writer merges in place (`data[sessionID] = string(value)` on the map it got from `Get`), so a shallow struct copy aliases the stored map and a writer that mutated the result without ever calling `Deploy` would still appear to have merged — a false pass the real API server cannot produce, since it hands back a freshly deserialized object;
   - `Deploy` stores the object it was handed under `key(namespace, name)`, replacing anything already there **including its `Data` map** — do not merge;
   - `Undeploy` deletes the entry; a missing entry is not an error;
   - `stored` returns the object and whether it exists, taking the same mutex;
   - `deployCalls` returns how many times `Deploy` has been called, taking the same mutex — the specs use it to prove the loop kept ticking.

   Add a compile-time assertion that the fake satisfies the interface, so a future signature change fails to compile rather than silently dropping the fake:

   ```go
   var _ k8s.ConfigMapDeployer = (*fakeCluster)(nil)
   ```

   Imports: `context`, `sync`, `errors` (standard library), `github.com/bborbe/k8s`, `corev1 "k8s.io/api/core/v1"`.

2. **Add the reader-contract helpers and the integration specs.** New file `pkg/heartbeat/integration_test.go`, `package heartbeat_test`. It uses the `TestSuite` already registered in `pkg/heartbeat/heartbeat_suite_test.go` — do NOT add a second suite bootstrap.

   Declare the two reader-contract helpers at the top of the file, with a comment stating exactly what they mirror:

   ```go
   // parseReaderEntry mirrors the reader's parse of one ConfigMap value:
   // json.loads, read refreshedAt, datetime.fromisoformat. The reader lives in
   // bborbe/claude-supervisor and is not modified; this mirrors its contract so a
   // drifting data shape fails a test here rather than silently disappearing from
   // the reader's view on the cluster.
   func parseReaderEntry(value string) (time.Time, error) {
       var entry struct {
           RefreshedAt string `json:"refreshedAt"`
       }
       if err := json.Unmarshal([]byte(value), &entry); err != nil {
           return time.Time{}, err
       }
       return time.Parse(time.RFC3339, entry.RefreshedAt)
   }

   // readerLive mirrors the reader's liveness verdict: the stamp is live when its
   // age is non-negative and below the TTL. An unparseable value is skipped by the
   // reader, so parseReaderEntry's error is the caller's to handle.
   func readerLive(refreshedAt time.Time, now time.Time, ttl time.Duration) bool {
       age := now.Sub(refreshedAt)
       return age >= 0 && age < ttl
   }
   ```

   Use `const readerTTL = 60 * time.Second` for the reader's TTL in these specs, with a comment naming it as the reader's fixed value (the writer never applies it).

   Then add `Describe("cluster heartbeat integration")` with these specs. Each spec builds its own `fakeCluster`, its own pinned `currentDateTime`, its own `heartbeat.NewActivityRecorder`, its own `heartbeat.NewConfigMapWriter(fake, k8s.Namespace("dev"), heartbeat.ConfigMapName, currentDateTime)` and its own `heartbeat.NewPublisher(recorder, writer, 10*time.Millisecond)`. Never share a fake across specs. Run the publisher in a `go func()` writing its result to a buffered channel (`go func()` is fine in `_test.go`), cancel the context to stop it, and `Eventually(done).Should(Receive(BeNil()))` — the publisher returns nil on cancel.

   a. **Stamps a live entry that parses through the reader's parser.** Pin the clock at `2026-10-05T12:00:00Z`; record `"abc"`; run the publisher; wait for the entry to appear with `Eventually(func() bool { _, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName); return ok }).Should(BeTrue())`; then read the object into a local (`stored, ok := fake.stored(...)`, `Expect(ok).To(BeTrue())`) and assert on `stored.Data["abc"]`:
   - `parseReaderEntry(value)` succeeds and yields exactly the pinned instant (compare with `Equal(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))`);
   - `readerLive(stamp, stamp, readerTTL)` is true, `readerLive(stamp, stamp.Add(61*time.Second), readerTTL)` is false — the boundary the spec's fifth acceptance criterion's arithmetic depends on — and `readerLive(stamp, stamp.Add(-1*time.Second), readerTTL)` is false (a future stamp: the spec's clock-skew failure mode's `age < 0` arm);
   - unmarshalling the value into `map[string]any` yields **exactly one** key, `refreshedAt` (the guard for "no other field is added" and "no credential reaches this store").

   b. **Merges, so another writer's entry survives.** Pre-seed the fake by calling its own `Deploy` with a `corev1.ConfigMap` for the same namespace and name whose `Data` already holds another writer's key — `"other"` mapped to the exact string `{"refreshedAt":"2026-10-05T11:00:00Z"}`. Then record `"abc"`, run the publisher, and wait with `Eventually` for the stored object to hold both keys. Assert:
   - `stored.Data` has key `"abc"` and key `"other"`;
   - `stored.Data["other"]` still equals the exact string that was seeded (this writer did not rewrite it).
   This is the end-to-end regression guard for the whole-object-replace semantics of the real `Deploy`: because the fake replaces `Data` wholesale, a writer that passed a fresh single-key ConfigMap to `Deploy` would lose `"other"` and fail here.

   c. **Stops refreshing a session that has been idle past the cutoff.** Pin the clock; record `"abc"`; advance the pinned clock by two minutes (`currentDateTime.SetNow(...)`, i.e. past `heartbeat.IdleCutoff`); run the publisher; then assert it never writes, across several ticks, with `Consistently(fake.deployCalls, 100*time.Millisecond, 10*time.Millisecond).Should(Equal(0))` — a `Consistently` over ten intervals is what proves "several ticks, no writes" without a fixed sleep. Also assert nothing was stored: `_, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)` is false. The session is no longer active, so no entry is written at all; the reader's own age arithmetic (covered by spec a) is what turns an abandoned entry into a dead one.

   d. **Keeps the entry live while the loop runs.** Use a **real** clock for this spec only (`libtime.NewCurrentDateTime()`, no `SetNow`): record `"abc"`, run the publisher with a `10 * time.Millisecond` interval, and `Eventually(fake.deployCalls).Should(BeNumerically(">=", 5))` so the loop has demonstrably ticked several times. Then read `stored.Data["abc"]`, parse it with `parseReaderEntry`, and assert `readerLive(stamp, time.Now(), readerTTL)` is true — the loop re-stamped recently enough that the reader still calls the worker live.

   Give each spec's `BeforeEach` a fresh `context.Background()` and a fresh `fakeCluster`. Specs a, b and c also build a fresh clock and pin it; spec d deliberately uses a real clock instead, because its whole point is that the running loop re-stamps recently enough.

   e. **The fake's `Get` does not alias the stored map.** Add a sibling `Describe("fakeCluster")` with one spec pinning the deep-copy the fake's `Get` must perform — without it the whole merge assertion in spec b is hollow, and a future edit that regressed the fake to a shallow copy would silently re-weaken the suite rather than fail it. Deploy a seeded ConfigMap through the fake, call `Get`, mutate the returned object's `Data`, then read the store back and assert the mutation did **not** reach it:

   ```go
   It("Get returns a copy that does not alias the stored map", func() {
       fake := newFakeCluster()
       Expect(fake.Deploy(ctx, corev1.ConfigMap{
           ObjectMeta: metav1.ObjectMeta{Name: heartbeat.ConfigMapName.String(), Namespace: "dev"},
           Data:       map[string]string{"other": "seed"},
       })).To(Succeed())
       got, err := fake.Get(ctx, k8s.Namespace("dev"), heartbeat.ConfigMapName)
       Expect(err).NotTo(HaveOccurred())
       got.Data["injected"] = "x"
       stored, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
       Expect(ok).To(BeTrue())
       Expect(stored.Data).NotTo(HaveKey("injected"))
   })
   ```

   This needs `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` added to the test file's imports (the other specs already import `corev1`).

3. **Do not change any production file.** This prompt adds test files only: `pkg/heartbeat/configmap_deployer_fake_test.go` and `pkg/heartbeat/integration_test.go`. Do not touch `pkg/heartbeat/*.go` production files, `main.go`, `pkg/factory/`, `mocks/`, `README.md` or `CHANGELOG.md`. The changelog entry is owned by the second prompt in this batch.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) the fake's `Deploy` replaces `Data` wholesale rather than merging, (ii) the merge spec would fail if the writer passed a single-key ConfigMap to `Deploy`, and (iii) no production file was modified.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Do NOT modify any production file. This prompt is test-only: `pkg/heartbeat/configmap_deployer_fake_test.go` and `pkg/heartbeat/integration_test.go` are the only new files, and nothing else changes.
- The fake's `Deploy` must replace the stored object wholesale, including its `Data` map. Do NOT make the fake merge `Data` — a merging fake would hide exactly the bug this test exists to catch.
- The fake is a hand-written in-memory implementation, not a counterfeiter mock: the project's counterfeiter mocks are stateless, and this test needs stored state to assert the merge and the shape. Do NOT add a counterfeiter annotation, and do NOT generate a fake into `mocks/`.
- Do NOT add a second Ginkgo suite bootstrap. `pkg/heartbeat/heartbeat_suite_test.go` (from the first prompt in this batch) already registers `TestSuite`; this file is another `package heartbeat_test` file in the same suite.
- Do NOT shell out to, copy or import `scripts/cluster-heartbeat.py` — it lives in `bborbe/claude-supervisor`, is not in this repository, and must not be modified. Reimplement its parse and verdict in Go as the two helpers described.
- Do NOT add a `replace` directive to `go.mod`, and do not hand-edit `go.mod`. Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command.
- Do NOT add a config knob, env var or `application` struct field. This prompt adds no production surface at all.
- Do NOT write the bearer token, any environment value, a pod name or a task id into the fake's store or into a log line. The entries in these specs are a session id and a timestamp and nothing else.
- Tests use Ginkgo v2 / Gomega, in the external `package heartbeat_test`. `go func()` is permitted in `_test.go`.
- Keep every file well under the repository's 2000-line `revive` limit and every function under the 80-line / 50-statement `funlen` limit.
</constraints>

<verification>
Run each of these and confirm the stated result. `make precommit` runs at the repository root; the `ROOTDIR=/workspace` prefix pins that root path explicitly, because this repository's `Makefile.variables` otherwise resolves `ROOTDIR` from a repository-root probe that is not reliable under the execution container.

1. `ROOTDIR=/workspace make test` — exits 0, and the new integration specs appear in the `pkg/heartbeat` run.
2. `ROOTDIR=/workspace make precommit` — exits 0.
3. `grep -c 'parseReaderEntry' pkg/heartbeat/integration_test.go` — prints a count greater than `0` (the reader's parse is mirrored in the test).
4. `grep -c 'refreshedAt' pkg/heartbeat/integration_test.go` — prints a count greater than `0` (the field the reader parses is asserted).
5. `grep -c 'readerLive' pkg/heartbeat/integration_test.go` — prints a count greater than `0` (the reader's age-versus-TTL verdict is asserted).
6. `grep -c 'var _ k8s.ConfigMapDeployer' pkg/heartbeat/configmap_deployer_fake_test.go` — prints `1` (the fake is compile-time checked against the interface).
7. `ls pkg/heartbeat/configmap_deployer_fake_test.go pkg/heartbeat/integration_test.go` — both paths exist.
</verification>
