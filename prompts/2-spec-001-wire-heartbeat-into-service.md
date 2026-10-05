---
status: draft
spec: [001-cluster-worker-heartbeat]
created: "2026-10-05T16:22:00Z"
branch: dark-factory/cluster-worker-heartbeat
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SETTLED — a failure to build the heartbeat at startup is logged and the service still serves,
   rather than failing to start. Two reasons: Desired Behavior 6 says a broken liveness path must
   not take prompt serving down, and the existing `runService` spec in `main_internal_test.go`
   constructs an `application` with no cluster access and must still pass — a fail-fast heartbeat
   would break that spec. The RBAC-missing failure mode is a *runtime write* failure, which the
   publisher already logs and skips.

2. SETTLED — the namespace is read from the pod's own service-account mount
   (`/var/run/secrets/kubernetes.io/serviceaccount/namespace`), not hardcoded to `dev` and not a new
   config knob. It is the same mount `k8s_rest.InClusterConfig` reads, so the two either both work
   or both fail, and the writer is guaranteed to publish into the namespace the pod runs in — which
   is the namespace the reader reads. No new `application` field, env var or flag is added.

3. SETTLED — composition uses `run.CancelOnFirstFinish`, not `run.CancelOnFirstErrorWait`. The
   `go-k8s-binary-conventions.md` guide's "Compose: HTTP + work loop" section names
   `CancelOnFirstFinish` for exactly this shape (a work loop alongside an HTTP-serving component):
   either side returning tears down the other, which is the clean-shutdown behaviour the spec
   wants on SIGTERM. `run` is not in this container's module cache, so the guide is the source for
   that call shape.

4. OUT OF SCOPE, REQUIRED COMPANION — the pod's RBAC to write the ConfigMap (create/get/update for
   the `claude-interactive` service account) is a manifest change in the config repo, not this
   repository. It must land with or before this image, or every write fails with
   `configmaps is forbidden` and the feature is inert while looking deployed. The spec names it as a
   required companion; the prompt records it as a constraint and asks the executor to note it in the
   report.
-->

# Wire the cluster heartbeat into the interactive service

<summary>
- The interactive service now publishes its own liveness while it is serving prompts.
- It resolves the cluster API from inside the pod and writes into the namespace the pod runs in — no new setting has to be supplied by the deployment.
- The heartbeat runs alongside the prompt-serving service and shuts down cleanly with it.
- A pod that cannot reach the cluster API still serves prompts; the liveness path is best-effort and its failure is logged, not fatal.
- The service's existing routes, authentication and response shapes are completely unchanged.
- The writer-side contract is documented in the README, so the shape and cadence outlive this change.
- The change is recorded in the changelog.
- The pod's permission to write the ConfigMap is a companion manifest change in the config repo and is explicitly not part of this change.
- The reader's own source is not modified.
</summary>

<objective>
Make the heartbeat live: assemble the writer, recorder and publisher from the previous prompt in this batch inside `runService`, resolve the in-cluster Kubernetes client and the pod's own namespace, wrap the service's session factory with the activity observer, and run the publisher concurrently with the interactive service so both shut down together. A failure to build the heartbeat is logged and the service still serves. End state: a service pod in nuke dev writes a fresh `claude-worker-heartbeats` entry per session it is serving, which `scripts/cluster-heartbeat.py --list --json` returns while the worker is served and stops returning within 60 seconds of its last refresh.
</objective>

<context>
This repository has no root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md` (the Definition of Done you are graded against; it forbids `replace` directives in `go.mod` and requires a `## Unreleased` changelog entry).

This prompt depends on the previous prompt in this batch (`1-spec-001-heartbeat-writer.md`), which created `pkg/heartbeat` with `NewActivityRecorder`, `NewObservingSessionFactory`, `NewConfigMapWriter`, `NewPublisher`, and the constants `ConfigMapName`, `RefreshInterval`, `IdleCutoff`. If that package does not exist, stop and report — do not create it yourself.

Files you will read and change (repo-relative):
- `main.go` — the `application` struct, `Run`, and `runService`. Read all three before editing. `runService` is the only function this prompt changes.
- `main_internal_test.go` — the specs for `application` and `runService`, in `package main` (not `main_test`) because `application` is unexported; the `RunSpecs` in `main_test.go` runs them.
- `pkg/heartbeat/` — the package the previous prompt landed. Read its exported surface before wiring it.
- `README.md` — has a "Service mode" paragraph and an "Env Vars" table.
- `CHANGELOG.md` — the newest section today is `## v0.6.0`; there is no `## Unreleased` section yet.
- `go.mod` — `github.com/bborbe/k8s` and the `k8s.io/*` modules are currently **indirect**. Do not hand-edit `go.mod`; `go mod tidy` (run by `make precommit`) promotes them once `main.go` imports them.

The library API you must call — read the landed source in the module cache before you write anything. These signatures are quoted from the landed files and are the contract; do not paraphrase them.

```go
// $(go env GOMODCACHE)/github.com/bborbe/k8s@v1.14.19/k8s_clientset.go
// package k8s — import path github.com/bborbe/k8s
// An empty kubeconfig means "in cluster": CreateConfig falls through to
// k8s_rest.InClusterConfig(), which reads the pod's service-account mount.
func CreateClientset(kubeconfig string) (Interface, error)
func CreateConfig(kubeconfig string) (*k8s_rest.Config, error)
```

```go
// $(go env GOMODCACHE)/github.com/bborbe/k8s@v1.14.19/k8s_configmap-deployer.go
// k8s.Interface embeds k8s_kubernetes.Interface, so the value CreateClientset
// returns is directly assignable to NewConfigMapDeployer's parameter.
func NewConfigMapDeployer(clientset k8s_kubernetes.Interface) ConfigMapDeployer
```

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.94.0/interactive/service.go
// package interactive — already imported as `interactive` in main.go
func NewServiceWithPermissions(
	sessions agentlib.SessionFactory,
	listen string,
	providerBaseURL string,
	registry *prometheus.Registry,
	auth Auth,
	permissions PermissionRegistry,
) Service
```

`pkg/heartbeat` (from the previous prompt in this batch) exports, and you must use exactly these:

```go
const ConfigMapName k8s.Name = "claude-worker-heartbeats"
const RefreshInterval = 20 * time.Second
const IdleCutoff = 90 * time.Second

func NewActivityRecorder(currentDateTime libtime.CurrentDateTimeGetter) ActivityRecorder
func NewObservingSessionFactory(delegate agentlib.SessionFactory, recorder ActivityRecorder) agentlib.SessionFactory
func NewConfigMapWriter(deployer k8s.ConfigMapDeployer, namespace k8s.Namespace, name k8s.Name, currentDateTime libtime.CurrentDateTimeGetter) Writer
func NewPublisher(recorder ActivityRecorder, writer Writer, interval time.Duration) Publisher
```

`Publisher.Run(ctx context.Context) error` blocks until the context is cancelled and returns nil; `Service.Run(ctx context.Context) error` behaves the same way.

The composition primitive — the canonical project pattern for running an HTTP/service goroutine alongside a work loop, from `go-k8s-binary-conventions.md`:

```go
return run.CancelOnFirstFinish(ctx,
    publisher.Run,          // the heartbeat work loop
    interactiveService.Run, // the interactive service
)
```

`run` is `github.com/bborbe/run`. `run.CancelOnFirstFinish` cancels every other function as soon as any one returns, and returns that first function's error — it does **not** join the others (waiting is `run.CancelOnFirstFinishWait`, which is not used here). That is exactly the clean-shutdown shape wanted: a SIGTERM makes the HTTP server return, which cancels the publisher, and one side failing tears down the other. The reference shape is documented at `/home/node/.claude/plugins/marketplaces/coding/docs/go-k8s-binary-conventions.md` ("Compose: HTTP + work loop via run.CancelOnFirstFinish"). Both `Publisher.Run` and `Service.Run` already have the `func(context.Context) error` shape it takes.

The pod's own namespace is read from the file the kubelet mounts at the standard service-account path — the same mount `k8s_rest.InClusterConfig()` reads the token and CA from, so the two either both work or both fail:

```
/var/run/secrets/kubernetes.io/serviceaccount/namespace
```

The heartbeat writer publishes into the namespace the pod runs in, which is the namespace the reader reads (`kubectlnukedev -n dev`). Do not hardcode `dev`, and do not add an env var or CLI flag for it.

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-k8s-binary-conventions.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Add the heartbeat to `runService` in `main.go`.** Replace the body of `runService` with this shape. Everything the existing function does is preserved; the additions are the clock, the recorder, the observer wrapper, and the composition of the publisher with the service.

   ```go
   func (a *application) runService(
       ctx context.Context,
       registry *prometheus.Registry,
       claudeEnv map[string]string,
   ) error {
       // The shape-scoped guard lives here rather than in a required:"true" struct tag:
       // argument.Parse evaluates required tags for every agent shape, so a tag would also
       // reject the task-routed jobs, which never serve HTTP. runService is reached only
       // when the agent shape is a service, so this is where the requirement belongs.
       if a.InteractiveAuthToken == "" {
           return errors.Errorf(
               ctx,
               "INTERACTIVE_AUTH_TOKEN is required for a service agent; a service that cannot authenticate must not serve unauthenticated",
           )
       }
       glog.V(2).Infof(
           "agent-claude service mode: serving readiness, metrics, prompt intake and the permission endpoint on %s",
           a.Listen,
       )
       // One registry, passed to the session factory and to the service together, so
       // a turn that pauses on a permission request is visible on the service's own
       // /permission route and answerable from outside the pod.
       permissions := interactive.NewPermissionRegistry()

       // The clock is created once here and shared by the activity recorder and the
       // heartbeat writer, so a session's activity and its entry's stamp come from the
       // same source of time.
       currentDateTime := libtime.NewCurrentDateTime()
       recorder := heartbeat.NewActivityRecorder(currentDateTime)
       // The observer wraps the Session the factory returns, not the factory's Create:
       // the library calls Create once per session and never again, so only Prompt can
       // report that a session is still being served.
       sessions := heartbeat.NewObservingSessionFactory(
           factory.CreateClaudeSessionFactory(
               a.ClaudeConfigDir,
               a.AgentDir,
               claudelib.ParseAllowedTools(a.AllowedToolsRaw),
               a.AnthropicModel,
               claudeEnv,
               permissions,
           ),
           recorder,
       )
       interactiveService := interactive.NewServiceWithPermissions(
           sessions,
           a.Listen,
           a.ProviderBaseURL,
           registry,
           interactive.NewAuthToken(a.InteractiveAuthToken),
           permissions,
       )

       clientset, err := k8s.CreateClientset("")
       if err != nil {
           // A broken liveness path must not take prompt serving down: the worker goes
           // unlisted rather than unserved. The next pod start retries the setup.
           glog.Warningf("cluster heartbeat disabled: %v", err)
           return interactiveService.Run(ctx)
       }
       publisher, err := buildHeartbeatPublisher(ctx, clientset, recorder, currentDateTime)
       if err != nil {
           // Same rule as above: the heartbeat is best-effort, prompt serving is not.
           glog.Warningf("cluster heartbeat disabled: %v", err)
           return interactiveService.Run(ctx)
       }
       return run.CancelOnFirstFinish(ctx, publisher.Run, interactiveService.Run)
   }
   ```

   The `clientset` is built here at the call site rather than inside `buildHeartbeatPublisher` so the helper takes it as a parameter — that seam is what lets a spec pass `k8smocks.K8sInterface` and exercise the helper's success path (the only `runService` spec takes the degrade branch, so without the seam the success path is untestable).

   Notes, each load-bearing:
   - Name the service variable `interactiveService`, not `service`: `github.com/bborbe/service` is already imported in `main.go` as `service`, and a local variable of that name shadows it.
   - The existing `glog.V(2)` line, the `permissions` registry, the session factory's six arguments and the `NewServiceWithPermissions` call keep their current values and order. Only the variable name of the service and the wrapping of the factory are new.
   - Do not add the heartbeat to the task-routed path in `Run`. A task-routed job does not serve HTTP and gains no writer.
   - Add the imports: `"github.com/bborbe/k8s"` (used by `buildHeartbeatPublisher` and `inClusterNamespace`), `heartbeat "github.com/bborbe/agent-claude/pkg/heartbeat"`, and `"github.com/bborbe/run"`. `goimports-reviser` (run by `make precommit`) orders them; do not hand-order.

2. **Add `buildHeartbeatPublisher` and the namespace reader to `main.go`.** Put both after `runService`. The helper builds the publisher from the in-cluster client, or returns an error the caller degrades on.

   ```go
   // buildHeartbeatPublisher builds the cluster heartbeat publisher from the
   // in-cluster client. An error means the pod cannot resolve the cluster API or its
   // own namespace; the caller treats that as "run without a heartbeat" rather than as
   // a startup failure, because the liveness path is best-effort and prompt serving is
   // not.
   func buildHeartbeatPublisher(
       ctx context.Context,
       clientset k8s.Interface,
       recorder heartbeat.ActivityRecorder,
       currentDateTime libtime.CurrentDateTimeGetter,
   ) (heartbeat.Publisher, error) {
       namespace, err := inClusterNamespace(ctx, inClusterNamespacePath)
       if err != nil {
           return nil, errors.Wrap(ctx, err, "resolve pod namespace")
       }
       writer := heartbeat.NewConfigMapWriter(
           k8s.NewConfigMapDeployer(clientset),
           namespace,
           heartbeat.ConfigMapName,
           currentDateTime,
       )
       return heartbeat.NewPublisher(recorder, writer, heartbeat.RefreshInterval), nil
   }

   // inClusterNamespacePath is where the kubelet mounts the pod's own namespace — the
   // same service-account mount k8s_rest.InClusterConfig reads from.
   const inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

   // inClusterNamespace reads the pod's own namespace from the file the kubelet mounts
   // at the standard service-account path. The heartbeat writer publishes into the
   // namespace the pod runs in — the same namespace the reader reads.
   func inClusterNamespace(ctx context.Context, path string) (k8s.Namespace, error) {
       content, err := os.ReadFile(path) // #nosec G304 -- path is a package constant
       if err != nil {
           return "", errors.Wrap(ctx, err, "read pod namespace")
       }
       namespace := strings.TrimSpace(string(content))
       if namespace == "" {
           return "", errors.Errorf(ctx, "pod namespace is empty in %s", path)
       }
       return k8s.Namespace(namespace), nil
   }
   ```

   `path` is a parameter rather than a literal inside the function so the reader can be exercised against a temp file; the `// #nosec G304` comment is required because `gosec` flags `os.ReadFile` on a variable path, and it mirrors the existing precedent in `cmd/run-task/main.go`. Keep that comment on the **same line** as the `os.ReadFile` call (that is where `gosec` looks for it) and keep the line short enough that `golines` does not wrap it onto the next line. Add the `strings` import.

3. **Update the specs in `main_internal_test.go`.** All specs live in the one `package main` suite; do not create a new test file.

   a. The three existing argument-parsing specs need **no change**.

   b. The existing `Describe("application.runService")` "serves until the context is cancelled, then returns nil" spec needs **no change to its assertions** — but add a comment to it saying that, with the heartbeat wiring, it also exercises the degrade path: the test process has no in-cluster service-account mount, so `k8s.CreateClientset("")` fails at the call site and `buildHeartbeatPublisher` is never reached; `runService` logs and serves anyway, and the spec passing is the evidence that a broken liveness path does not stop prompt serving.

   c. The existing "fails to start when no interactive auth token is configured" spec needs **no change**: the guard still runs first, before anything is constructed.

   d. Add a `Describe("inClusterNamespace")` with three specs, driving the real boundary:
   - reads the namespace and trims the trailing newline: write `"dev\n"` into a file under `GinkgoT().TempDir()` with `os.WriteFile(path, []byte("dev\n"), 0600)`, then expect `inClusterNamespace(context.Background(), path)` to return `k8s.Namespace("dev")` with no error;
   - returns an error when the file is absent: pass a path under `GinkgoT().TempDir()` that was never written;
   - returns an error when the file holds only whitespace: write `"  \n"` and expect an error.

   e. Add a `Describe("buildHeartbeatPublisher")` with two specs, exercising the helper both ways now that it takes the clientset as a parameter (the only `runService` spec takes the degrade branch, so without these the helper's paths are untested):
   - **success:** write `"dev\n"` into a file under `GinkgoT().TempDir()`, then call `buildHeartbeatPublisher(ctx, k8smocks.K8sInterface{}, recorder, libtime.NewCurrentDateTime())` with the namespace path pointed at that file (pass the path in, or use a package-level `var` seam if the helper reads the constant — do not add a config knob for it), and expect a **non-nil** `heartbeat.Publisher` and a **nil** error;
   - **failure:** point the namespace path at a file that was never written and expect a **nil** `heartbeat.Publisher` and a **non-nil** error.

   Add the `github.com/bborbe/k8s` and `path/filepath` imports.

4. **Add the writer-side contract to `README.md`.** Add a short subsection after the existing "Service mode" paragraph. It must state the contract, because it is domain knowledge that outlives this change:

   ```markdown
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
   ```

   Keep the edit to that one addition; do not restructure the README or add env vars (this change adds none).

5. **Add a `## Unreleased` section to `CHANGELOG.md`.** There is none today — the newest section is `## v0.6.0`. Insert `## Unreleased` above it, matching the existing bullet shape (a `- feat:` line with a prose explanation after an em dash):

   ```
   ## Unreleased

   - feat: a service agent publishes its own cluster liveness — every 20 seconds it stamps one entry per session it has served within the last 90 seconds into the `claude-worker-heartbeats` ConfigMap (key: the session id, value: `{"refreshedAt": "<RFC3339>"}`), so the cluster liveness reader in `bborbe/claude-supervisor` returns a live cluster worker while it is served and stops returning it within 60 seconds of its last refresh; the write merges into the ConfigMap's existing data, a failing write is logged without taking prompt serving down, and the pod's own namespace is resolved from the service-account mount rather than configured
   ```

   Do not touch any released section. This is the only prompt in the batch that writes a changelog entry — the previous and next prompts must not add one.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each one passes, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `/readiness`, `/metrics`, `/prompt` and the permission endpoint keep their current behavior, authentication and response shapes, (ii) a service pod with no reachable cluster API still serves, and (iii) no new `application` struct field, env var or CLI flag was added.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- The service's existing surface is unchanged: `/readiness`, `/metrics`, `/prompt` and the permission endpoint keep their current behavior, their authentication and their response shapes. This prompt adds a background writer and touches nothing about routing.
- The task-routed job path (Kafka jobs) does not serve HTTP and gains no writer. `Run`'s task branch is unchanged.
- A failing cluster write must not take the service down, and neither must a failure to build the heartbeat at startup: log at `Warningf` and continue serving. The liveness path is best-effort.
- Do NOT add a new `application` struct field, env var or CLI flag. The ConfigMap name, the cadence, the idle cutoff and the namespace are all derived, not configured.
- Do NOT hardcode the namespace (`dev` or otherwise). Read it from the pod's service-account mount.
- Do NOT write the bearer token, any environment value, a pod name or a task id into the ConfigMap or into a log line. The `glog` lines in this prompt print the listen address, the session id and errors — never the token.
- The reader's own source is unmodified and is not in this repository: `scripts/cluster-heartbeat.py` lives in `bborbe/claude-supervisor`. Do not create it here, do not copy it here, and do not modify it.
- Do NOT modify `github.com/bborbe/agent`. `Session` and `SessionFactory` are frozen external contracts; this prompt only wraps the factory's return value.
- Do NOT edit `pkg/heartbeat/` in this prompt — it is the previous prompt's deliverable. If a symbol you need is missing there, stop and report rather than adding it here.
- `cmd/run-task/main.go` is the local file-based CLI mode and is out of scope: it does not serve HTTP, gains no heartbeat, and does not import `pkg/heartbeat`. It must still compile untouched. This prompt changes no exported signature, so it needs no change.
- Do NOT add a `replace` directive to `go.mod` — `docs/dod.md` forbids it (it breaks remote install). Do not hand-edit `go.mod` at all: `go mod tidy` (run by `make precommit`) promotes `github.com/bborbe/k8s` and the `k8s.io/*` modules to direct.
- Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command. `vendor/` is a build-time artifact, gitignored, and wiped by `make precommit`.
- The pod's permission to write the ConfigMap is a manifest change in the config repo (`configmaps` create/get/update for the `claude-interactive` service account). It is explicitly out of scope here and must land with or before this image. Note it in your final report.
- Errors use `github.com/bborbe/errors` — `errors.Wrap(ctx, err, "msg")`, `errors.New(ctx, "msg")`, `errors.Errorf(ctx, "fmt", ...)`. Never `fmt.Errorf`, never a bare `return err`.
- Logging uses `github.com/golang/glog`: `V(2)` for the service-mode line, `Warningf` for a degraded heartbeat.
- Tests use Ginkgo v2 / Gomega, in the existing `package main` suite.
- Keep every function under the 80-line / 50-statement `funlen` limit; `runService` and its helpers must each stay well under it.
</constraints>

<verification>
Run each of these and confirm the stated result. `make precommit` runs at the repository root; the `ROOTDIR=/workspace` prefix pins that root path explicitly, because this repository's `Makefile.variables` otherwise resolves `ROOTDIR` from a repository-root probe that is not reliable under the execution container.

1. `ROOTDIR=/workspace make test` — exits 0.
2. `ROOTDIR=/workspace make precommit` — exits 0.
3. `grep -c 'heartbeat.NewObservingSessionFactory' main.go` — prints `1`.
4. `grep -c 'run.CancelOnFirstFinish(ctx, publisher.Run, interactiveService.Run)' main.go` — prints `1`.
5. `grep -c 'heartbeat.NewConfigMapWriter' main.go` — prints `1`.
6. `grep -c 'inClusterNamespacePath' main.go` — prints a count greater than `0`.
7. `grep -c 'cluster heartbeat disabled' main.go` — prints `2` (both degrade branches — the in-cluster client failure and the publisher-build failure — are present).
8. `grep -n '^## Unreleased' CHANGELOG.md` — prints exactly one line.
9. `grep -c 'claude-worker-heartbeats' README.md` — prints a count greater than `0` (the writer-side contract is documented).
10. `grep -c 'github.com/bborbe/k8s' go.mod` — prints a count greater than `0`, and that line is **not** marked `// indirect`.
11. `grep -c 'InteractiveAuthToken' main.go` — prints a count greater than `0` (the auth guard is preserved).
12. `go build ./...` — succeeds (the new in-cluster k8s client dependency compiles).
</verification>
