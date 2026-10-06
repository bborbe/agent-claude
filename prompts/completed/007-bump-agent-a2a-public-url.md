---
status: completed
summary: Bumped github.com/bborbe/agent to v0.96.0 and wired the A2A public address (A2A_PUBLIC_URL) through the application argument struct into interactive.NewServiceWithPermissions, adding a service-shape startup guard, an Agent Card contract spec, and README/CHANGELOG updates
execution_id: agent-claude-a2a-exec-007-bump-agent-a2a-public-url
dark-factory-version: dev
created: "2026-10-06T07:16:03Z"
queued: "2026-10-06T07:16:03Z"
started: "2026-10-06T07:16:46Z"
completed: "2026-10-06T07:22:13Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. SETTLED — the address is bound through the `application` argument struct, NOT read with
   `interactive.A2APublicURLFromEnv`. The library ships that accessor (and its own doc recommends
   it), but it calls `os.Getenv` directly, which bypasses `argument.Parse` (precedence, validation,
   `ParseAndPrint` logging). The go-k8s-binary-conventions rule `argument-struct-not-os-getenv`
   and its "use struct fields, not os.Getenv" section reject exactly that in a binary that already binds its configuration through a struct. This
   mirrors how prompt 002 bound `INTERACTIVE_AUTH_TOKEN` with
   `interactive.NewAuthToken(a.InteractiveAuthToken)` rather than `interactive.AuthFromEnv`.

2. SETTLED — the field is `required:"false"` with the requirement enforced in `runService`, for
   the same reason prompt 002 recorded for the token: `argument.Parse` evaluates `required:"true"`
   for every agent shape, so a tag would make `A2A_PUBLIC_URL` mandatory for the task-routed Kafka
   jobs too, which never serve HTTP.

3. SETTLED — no `display` tag. The address is public by design (it is the URL the Agent Card
   advertises to anyone), so printing it in the startup log is correct and useful.

4. WHY THE CONTRACT SPEC (req 6, last bullet) — `listen`, `providerBaseURL` and `publicURL` are all
   `string`, so a misordered constructor call compiles. An audit proved that swapping
   `a.A2APublicURL` and `a.ProviderBaseURL` still passed every test and every grep in the first
   draft of this prompt; in production the card would then advertise the provider endpoint. The
   contract spec serves the real Agent Card route and asserts the advertised URL, which is the
   only check that tells the two orders apart.

5. OUT OF SCOPE — the Agent Card's `name` (`"interactive"`), `version` and skill are constants
   inside `github.com/bborbe/agent/interactive`. This binary cannot change them; a different card
   name is a library change.

6. OUT OF SCOPE — the Kubernetes manifest change that supplies `A2A_PUBLIC_URL` to the
   `claude-interactive` pod lives in the config repo. It must land with or before the image bump,
   or the service pod refuses to start on the new guard.
-->

# Bump github.com/bborbe/agent to v0.96.0 and wire the A2A public address

<summary>
- The interactive service in the `agent-claude` image gains the A2A surface the shared agent library now provides: a public Agent Card and an authenticated A2A endpoint.
- The address the Agent Card advertises comes from deployment configuration, never derived from the listen address.
- A service pod that has no public address configured refuses to start, instead of advertising an empty endpoint.
- The address is bound through this binary's own configuration struct, so it gets the same parsing and startup logging as every other setting — not read ad hoc from the environment.
- A test serves the real Agent Card and checks it advertises the configured address, so a mix-up between the service's several address settings cannot ship unnoticed.
- The shared agent library is bumped to the release that added the A2A surface.
- The task-routed Kafka job path is completely unchanged — it never serves HTTP, so it does not need the address.
- The changelog and the README's environment-variable table and service-mode description are updated.
- Adding the Kubernetes manifest that supplies the address is explicitly not part of this change.
</summary>

<objective>
Wire the A2A surface that `github.com/bborbe/agent` v0.96.0 adds into the service this binary runs, by bumping the library and passing it the externally reachable address the Agent Card must advertise. End state: a service pod serves an Agent Card whose advertised endpoint is the configured `A2A_PUBLIC_URL`, and a service pod with no `A2A_PUBLIC_URL` fails to start. The task-routed job path behaves exactly as it does today.
</objective>

<context>
This repository has no root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md`, the Definition of Done you are graded against (it forbids adding `replace` or `exclude` directives to `go.mod` — the existing `exclude` block predates this prompt and stays untouched — and requires a `## Unreleased` changelog entry).

Coding guides, available in the container:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-k8s-binary-conventions.md` — the argument-struct rule this prompt follows
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`

Files you will read and change (repo-relative):
- `main.go` — the `application` argument struct (find the `InteractiveAuthToken` field and its doc comment) and `runService` (find the `InteractiveAuthToken == ""` guard, the `glog.V(2).Infof("agent-claude service mode: serving …")` line, and the `interactive.NewServiceWithPermissions(` call). Read both before editing. `main.go` already imports `agentlib "github.com/bborbe/agent"`, `interactive "github.com/bborbe/agent/interactive"` and `github.com/prometheus/client_golang/prometheus`.
- `go.mod` — the current pin is `github.com/bborbe/agent v0.94.0` in the direct require block.
- `main_internal_test.go` — `package main` (not `main_test`) because `application` is unexported. It holds the argument-binding specs (find `binds INTERACTIVE_AUTH_TOKEN into the struct field`) and the `Describe("application.runService"` block, which contains one spec that starts the service and the missing-token spec `fails to start when no interactive auth token is configured`. The `RunSpecs` in `main_test.go` runs them.
- `CHANGELOG.md` — the newest section today is `## v0.7.0`; there is no `## Unreleased` section yet.
- `README.md` — has a "Service mode" paragraph and an "Env Vars" table (find the `INTERACTIVE_AUTH_TOKEN` row).

The library API you must call — read the landed source in the module cache after the bump; this is quoted from it and is the contract:

```go
// $(go env GOMODCACHE)/github.com/bborbe/agent@v0.96.0/interactive/service.go
// publicURL is the sixth parameter: after auth, before permissions.
func NewServiceWithPermissions(
	sessions agentlib.SessionFactory,
	listen string,
	providerBaseURL string,
	registry *prometheus.Registry,
	auth Auth,
	publicURL string,
	permissions PermissionRegistry,
) Service
```

The library's `docs/interactive-service.md` describes `publicURL` as *"the externally reachable address the Agent Card advertises, verbatim … never derived from `listen` or from a request header."* The same paragraph recommends building it with `interactive.A2APublicURLFromEnv(ctx)` — **this binary deliberately does not**, because that function reads the environment directly instead of going through the argument struct (see `<constraints>`). Expect that conflict and follow the constraint.

The Agent Card route is `GET /.well-known/agent-card.json`. It is exempt from the bearer-token gate and does not touch the session factory, so it can be served in a test with a stub factory and no `Authorization` header.
</context>

<requirements>
1. Bump the library: run `go get github.com/bborbe/agent@v0.96.0`, then `go mod tidy`. Confirm `go.mod` now pins `github.com/bborbe/agent v0.96.0` and carries no `replace` directive. Leave the pre-existing `exclude` block exactly as it is — it predates this change. If `v0.96.0` does not resolve, stop and report — do not add a `replace`, and do not fall back to a pseudo-version.

2. In `main.go`, add a field to the `application` struct directly after `InteractiveAuthToken`, with a doc comment in the same style as its neighbours:

   ```go
   // A2APublicURL is the externally reachable address of the interactive service's A2A
   // endpoint, advertised verbatim in its Agent Card. It is configuration, never derived from
   // Listen, so the card cannot advertise the listen address. Required for a service agent;
   // enforced in runService rather than by a required tag, for the same reason as
   // InteractiveAuthToken.
   A2APublicURL string `required:"false" arg:"a2a-public-url" env:"A2A_PUBLIC_URL" usage:"Externally reachable A2A endpoint URL the Agent Card advertises (service agents only)"`
   ```

3. In `runService`, directly after the existing `InteractiveAuthToken == ""` guard, add a second shape-scoped guard:

   ```go
   if a.A2APublicURL == "" {
   	return errors.Errorf(
   		ctx,
   		"A2A_PUBLIC_URL is required for a service agent; the Agent Card must advertise a real network endpoint",
   	)
   }
   ```

4. Move the constructor call out of `runService` into its own method, so it can be tested against the real Agent Card route, and pass the address as the new sixth argument — after the auth argument and before `permissions`:

   ```go
   // newInteractiveService builds the interactive HTTP surface from the application's
   // configuration. The three address settings are all strings, so their order is checked
   // by a spec that serves the Agent Card rather than by the compiler.
   func (a *application) newInteractiveService(
   	sessions agentlib.SessionFactory,
   	registry *prometheus.Registry,
   	permissions interactive.PermissionRegistry,
   ) interactive.Service {
   	return interactive.NewServiceWithPermissions(
   		sessions,
   		a.Listen,
   		a.ProviderBaseURL,
   		registry,
   		interactive.NewAuthToken(a.InteractiveAuthToken),
   		a.A2APublicURL,
   		permissions,
   	)
   }
   ```

   In `runService`, replace the inline `interactive.NewServiceWithPermissions(…)` call with `interactiveService := a.newInteractiveService(sessions, registry, permissions)`. Everything else in `runService` stays as it is.

5. Update the `glog.V(2).Infof("agent-claude service mode: serving …")` line in `runService` so it also names the A2A Agent Card and endpoint among what it serves.

6. Tests in `main_internal_test.go`:
   - **Binding spec.** Next to `binds INTERACTIVE_AUTH_TOKEN into the struct field`, add a spec that sets `A2A_PUBLIC_URL`, parses, and asserts `app.A2APublicURL` equals the value. Restore the environment with `DeferCleanup`, exactly as the neighbouring spec does.
   - **Existing start spec.** The one spec under `Describe("application.runService"` that builds an `application` and expects the service to start must now also set `A2APublicURL: "https://agent.example.test/a2a"`, or it fails on the new guard.
   - **Tighten the missing-token spec.** Change `fails to start when no interactive auth token is configured` from `Expect(err).To(HaveOccurred())` to `Expect(err).To(MatchError(ContainSubstring("INTERACTIVE_AUTH_TOKEN")))`, so with two guards it still proves *which* one fired.
   - **Missing-address spec.** Beside it, add `fails to start when no A2A public URL is configured`: an `application` with `Listen: "127.0.0.1:0"` and `InteractiveAuthToken: "test-token"` but no `A2APublicURL`. Run `runService` in a goroutine with a cancellable context (cancel it in `DeferCleanup`), send its error on a channel, and assert `Eventually(done, 5*time.Second).Should(Receive(MatchError(ContainSubstring("A2A_PUBLIC_URL"))))`. Running it in a goroutine matters: if the guard were missing, a synchronous call would start serving and block until the suite timeout.
   - **Agent Card contract spec.** Add `Describe("application.newInteractiveService")` with one spec. Build an `application` with `Listen: "127.0.0.1:0"`, `ProviderBaseURL: "http://provider.example.test"`, `InteractiveAuthToken: "test-token"` and `A2APublicURL: "https://agent.example.test/a2a"`. Call `app.newInteractiveService(&agentmocks.SessionFactory{}, prometheus.NewRegistry(), interactive.NewPermissionRegistry())`, importing `agentmocks "github.com/bborbe/agent/mocks"`, `interactive "github.com/bborbe/agent/interactive"`, `net/http` and `net/http/httptest`. Serve `httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)` through `.Handler()` into an `httptest.NewRecorder()` with **no** `Authorization` header. Assert the status is `200`, the body contains `https://agent.example.test/a2a`, and the body contains neither `provider.example.test` nor `127.0.0.1`.

7. `CHANGELOG.md`: add a `## Unreleased` section above `## v0.7.0` with one bullet starting `feat:` — bump `github.com/bborbe/agent` to v0.96.0 and wire the A2A public address (`A2A_PUBLIC_URL`, required for a service agent, bound through the argument struct, never derived from the listen address).

8. `README.md`:
   - Add an `A2A_PUBLIC_URL` row to the "Env Vars" table directly after the `INTERACTIVE_AUTH_TOKEN` row, in the same column shape (`yes (when AGENT_TYPE=service)`, no default, a one-line description).
   - In the "Service mode" paragraph, add the A2A Agent Card (`/.well-known/agent-card.json`) and the `/a2a` endpoint to the list of what the service serves.
   - In the same paragraph, the sentence "Every route except readiness and metrics requires `Authorization: Bearer <token>`" is **wrapped across two lines** (the line break falls after "Every"). Change it to read "Every route except readiness, metrics and the Agent Card requires `Authorization: Bearer <token>`", so it stays true now that the card is exempt. Re-wrap the paragraph as needed; the verification check joins lines before matching.
   - Add one sentence stating that a service pod with no `A2A_PUBLIC_URL` fails to start.

9. Self-check: re-run every command in `<verification>` and confirm each passes; then walk each bullet of `<summary>` against the diff.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT call `interactive.A2APublicURLFromEnv` or `os.Getenv` for the address. The address must be bound through the `application` struct, like every other setting this binary reads — even though the library's own doc recommends the accessor.
- Do NOT add `required:"true"` to the new field — it would make the address mandatory for the task-routed jobs, which never serve HTTP.
- Do NOT change the task-routed job path (`Run` for non-service shapes) in any way.
- Do NOT add `replace` or `exclude` directives to `go.mod`, and do not pin a pseudo-version.
- Do NOT touch the Agent Card's name, version or skill — they are library constants, out of scope here.
- Do NOT add a Kubernetes manifest — the deployment that supplies `A2A_PUBLIC_URL` lives in another repository.
- Errors use `github.com/bborbe/errors` (`errors.Wrap(ctx, err, …)`, `errors.Errorf(ctx, …)`), never `fmt.Errorf` and never a bare `return err`.
- Tests are Ginkgo v2 / Gomega; counterfeiter mocks come from the `mocks` packages.
- Existing tests must still pass.
</constraints>

<verification>
- `ROOTDIR=/workspace make precommit` — exits 0.
- `grep -n 'bborbe/agent v0.96.0' go.mod` — prints one line.
- `! grep -q '^replace' go.mod` — exits 0.
- `grep -c '^exclude' go.mod` — prints `1` (the pre-existing block, unchanged).
- `! grep -q 'A2APublicURLFromEnv\|os.Getenv("A2A_PUBLIC_URL")' main.go` — exits 0.
- `grep -n 'a.A2APublicURL,' main.go` — prints the constructor argument line.
- `grep -n 'agent-card.json' main_internal_test.go` — prints the contract spec's request line.
- `grep -n 'A2A_PUBLIC_URL' README.md CHANGELOG.md main_internal_test.go` — each file appears at least once.
- `grep -n 'agent-card.json' README.md` — prints a line.
- `tr '\n' ' ' < README.md | tr -s ' ' | grep -q 'Every route except readiness, metrics and the Agent Card requires'` — exits 0 (lines are joined first, because the paragraph wraps).
- `! (tr '\n' ' ' < README.md | tr -s ' ' | grep -q 'except readiness and metrics requires')` — exits 0 (the old, now-false sentence is gone).
- `awk '/^## /{sec=$0} /v0.96.0/{print sec; exit}' CHANGELOG.md` — prints `## Unreleased`.
</verification>
