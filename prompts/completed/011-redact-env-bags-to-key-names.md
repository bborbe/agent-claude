---
status: completed
summary: Added pkg/envbag.KeyValueList so the startup log renders CLAUDE_ENV and ENV_CONTEXT as sorted key names only, wired into both main packages, with parser-contract, package, and regression specs plus a changelog entry.
execution_id: agent-claude-redact-env-bags-exec-011-redact-env-bags-to-key-names
dark-factory-version: v0.196.0
created: "2026-10-06T20:47:14Z"
queued: "2026-10-06T20:47:14Z"
started: "2026-10-06T20:48:10Z"
completed: "2026-10-06T20:57:09Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. FOLLOW-UP TO v0.9.1. That release redacted `AnthropicAuthToken`. Its PR review (bborbe/agent-claude#26)
   flagged a residual leak, verified at source: `CLAUDE_ENV` and `ENV_CONTEXT` are comma-separated
   KEY=VALUE bags with no redaction, and `buildClaudeEnv` copies `ANTHROPIC_AUTH_TOKEN` out of
   `ClaudeEnvRaw` whenever the dedicated field is empty — so a deployment supplying the token through
   `CLAUDE_ENV` still prints it verbatim at every start.

2. APPROACH IS THE OPERATOR'S CHOICE (2026-10-06): print the bags' KEY NAMES only, never their values.
   Two alternatives were offered and declined:
   - `display:"length"` on the bag — collapses the whole bag to a character count and destroys the
     diagnostic of which variables a pod received.
   - `envparse.RedactForLog` (github.com/bborbe/agent/envparse) — keeps values unless the key name
     contains a marker such as TOKEN or SECRET. A heuristic: it misses any secret stored under an
     innocuous key. Key-names-only cannot miss, by construction.

3. WHY A NAMED TYPE, NOT A TAG. The argument printer honours only `display:"hidden"` and
   `display:"length"`; there is no "keys" mode. But its default branch formats the field with `%v`,
   which calls `String()` on a type implementing fmt.Stringer — so a named string type whose
   `String()` returns the key set is rendered as keys. The parser accepts named string types
   (`argument_env.go` / `argument_args.go`: `PkgPath() != "" && Kind() == reflect.String`), and this
   repo already uses several (`claudelib.ClaudeModel`, `base.Branch`).

4. KNOWN LIMIT OF THE FORMAT, NOT OF THIS FIX. A value containing a comma (`TOKEN=ab,cd=ef`) is
   already mis-parsed at runtime as two pairs, so `String()` prints `TOKEN,cd` — `cd` being a fragment
   of the intended value. Out of scope here; the bag format has no escaping.
-->

# Redact the CLAUDE_ENV and ENV_CONTEXT bags to their key names in the startup log

<summary>
- The startup configuration log stops printing the values held in the two comma-separated environment settings, which can carry a credential.
- It still prints which variable names each setting supplied, so an operator can confirm what a pod received.
- A credential passed through either setting is never written to the log, regardless of what its variable is called.
- Both binaries in this repository — the Kafka job entry point and the local CLI — behave the same way.
- The values themselves reach the Claude process exactly as before; only what is logged changes.
- A test proves a planted secret value never appears in the log while its variable name does.
- The change is recorded in the changelog.
</summary>

<objective>
Stop this binary writing the values of `CLAUDE_ENV` and `ENV_CONTEXT` into its startup configuration log, because either can carry a credential — `buildClaudeEnv` sources `ANTHROPIC_AUTH_TOKEN` from `CLAUDE_ENV` when the dedicated field is empty. End state: the log shows each bag's key names only, and every runtime consumer of the bags receives the same key/value pairs it receives today.
</objective>

<context>
This repository has no tracked root `CLAUDE.md` — the conventions you must follow are restated in `<constraints>`. Read `docs/dod.md` (the Definition of Done you are graded against).

How the startup log is produced: `main()` calls `service.Main` (github.com/bborbe/service), which calls `argument.ParseAndPrint` (github.com/bborbe/argument/v2). The printer honours only `display:"hidden"` and `display:"length"`; for every other field it logs `Argument: <Name> '<%v of the value>'`. `%v` calls `String()` when the value implements `fmt.Stringer`. The parser fills named string types (types whose underlying kind is `string`), so a field of a named string type is parsed exactly like a plain `string`.

Files you will read and change (repo-relative):
- `main.go` — the `application` struct declares `EnvContextRaw` and `ClaudeEnvRaw` as plain `string`. They are consumed by `envparse.KeyValuePairs(a.EnvContextRaw)` (where the prompt context is built) and `envparse.KeyValuePairs(a.ClaudeEnvRaw)` inside `buildClaudeEnv`.
- `cmd/run-task/main.go` — its own `application` struct declares the same two fields, consumed the same two ways.
- `main_internal_test.go` — `package main` specs. The `application.buildClaudeEnv` specs construct the struct with `ClaudeEnvRaw` string literals. The `application startup argument log` spec (added in v0.9.1) is the pattern for capturing the printer's output: redirect the stdlib logger to a buffer, call `argument.Print`, assert on the buffer, restore with `DeferCleanup`.
- `pkg/heartbeat/` — the pattern for a package under `pkg/`: one `<name>_suite_test.go` bootstrapping Ginkgo, specs in external `_test` packages.
- `CHANGELOG.md` — add an `## Unreleased` section above the newest released section.

`envparse.KeyValuePairs(raw string) map[string]string` is in `github.com/bborbe/agent/envparse` (already a dependency). Read it in the module cache at the version `go.mod` pins before relying on its behaviour.

Coding guides (in-container paths — read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-build-args-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-k8s-binary-conventions.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md`
</context>

<requirements>
1. **Add a named string type for a KEY=VALUE bag in a new package under `pkg/`** named `pkg/envbag`, type `KeyValueList`. Both `main` packages must import it, which is why it cannot live in either of them. It must:
   - have underlying type `string`, so the argument parser fills it unchanged;
   - implement `fmt.Stringer` with a **value receiver**, returning only the key names of the parsed pairs, **sorted** (deterministic output), joined with `,` (no space) — never any value. Derive the keys from the same parse the runtime uses, never by re-splitting the raw string. An empty bag renders as the empty string;
   - expose a method returning the parsed `map[string]string`, delegating to `envparse.KeyValuePairs`, so every consumer receives exactly the pairs it receives today;
   - carry doc comments on the type and every exported method stating that `String()` is the log rendering and deliberately omits values.

2. **Change the type of `EnvContextRaw` and `ClaudeEnvRaw` to the new type in both `main.go` and `cmd/run-task/main.go`.** Leave their `required`, `arg`, `env` and `usage` tags exactly as they are, and add no `display` tag. Replace each `envparse.KeyValuePairs(a.<field>)` call with the new parsing method, so runtime behaviour is unchanged.

3. **Leave the existing `buildClaudeEnv` specs as they are** in `main_internal_test.go` — their `ClaudeEnvRaw: "..."` literals are untyped string constants, which Go assigns to a named string type unchanged, so they compile and assert the same maps without edits. Do not weaken any existing assertion. **Then add a parser contract spec per bag** inside the existing `application argument parsing` Describe (its `BeforeEach` already resets `flag.CommandLine`), following the `binds INTERACTIVE_AUTH_TOKEN into the struct field` spec: `os.Setenv("CLAUDE_ENV", "A=1,B=2")` (resp. `ENV_CONTEXT`) with a `DeferCleanup` unset, call `argument.Parse(ctx, app)`, and assert the field's parsing method returns `map[string]string{"A": "1", "B": "2"}`. This proves the argument parser fills the new named type from the env var — the contract the whole fix rests on.

4. **Add specs for the new package**, in its own Ginkgo suite following the `pkg/heartbeat` layout: `String()` returns sorted key names and no value; the empty bag renders as the empty string; the parsing method returns the same map as `envparse.KeyValuePairs` for the same input (including `nil` for the empty bag); `OK=yes,stray-fragment` renders as `OK` only (an entry without `=` may be a split-off value fragment and must never print); `EQ=a=b=c` renders as `EQ`; ` B = 2 , A = 1 ` renders as `A,B`.

5. **Add a regression spec in `main_internal_test.go`** next to the `application startup argument log` spec, using the same capture technique. Give `ClaudeEnvRaw` the content `ANTHROPIC_AUTH_TOKEN=<sentinel-1>,FOO=bar` and `EnvContextRaw` the content `INNOCUOUS_NAME=<sentinel-2>`, where each sentinel is a distinctive string. Call `argument.Print`. Declare both sentinels as `const`, so the field literals stay untyped constants and need no conversion. Assert the output contains the exact substrings `Argument: ClaudeEnvRaw 'ANTHROPIC_AUTH_TOKEN,FOO'` and `Argument: EnvContextRaw 'INNOCUOUS_NAME'`, and contains **neither** sentinel. The second bag deliberately uses a key name no marker heuristic would flag — this is the case that distinguishes key-names-only from `envparse.RedactForLog`, and the spec must cover it.

6. **Add an `## Unreleased` section to `CHANGELOG.md`** above the newest released section, matching the existing bullet shape:

   ```
   ## Unreleased

   - fix: the `CLAUDE_ENV` and `ENV_CONTEXT` bags are no longer printed with their values by the startup configuration log — either can carry a credential (`ANTHROPIC_AUTH_TOKEN` is read from `CLAUDE_ENV` when the dedicated field is empty), and both were logged verbatim; each now renders as its sorted key names only, so the log still shows which variables a pod received
   ```

   Do not touch any released section.

**Self-check before finishing.** Re-run every command in `<verification>` and confirm each passes, then walk each numbered requirement against the change you made. In particular confirm (i) no `envparse.KeyValuePairs(a.` call remains in either `main` package; (ii) the runtime maps are unchanged — the existing `buildClaudeEnv` specs still pass without their expectations edited; (iii) the requirement-5 spec genuinely fails without the fix — prove it by temporarily changing `String()` to return the raw value, running the spec, and restoring.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass, and no existing assertion may be weakened.
- Do NOT change `github.com/bborbe/argument` or `github.com/bborbe/agent` — fix the call site. No `replace` directive in `go.mod`, no vendored fork.
- Do NOT use `display:"length"` or `display:"hidden"` on the two bags, and do NOT use `envparse.RedactForLog` — the operator chose key-names-only; see `<objective>`.
- Do NOT change the env var names `CLAUDE_ENV` and `ENV_CONTEXT` or their `arg` names — they are what deployments supply.
- Do NOT change what the Claude process receives — only the log rendering changes.
- Do NOT touch `AnthropicAuthToken`, `InteractiveAuthToken`, `SentryDSN` or `SentryProxy`.
- Do NOT run `go mod vendor` and do NOT write `-mod=vendor` in any command.
- Errors use `github.com/bborbe/errors` — never `fmt.Errorf`, never a bare `return err`.
- Exported types and methods carry doc comments.
- Tests use Ginkgo v2 / Gomega in external `_test` packages, except `main_internal_test.go`, which is `package main` by necessity.
- Run `make precommit` at the repository root only; `cmd/run-task/Makefile` is a local-run helper with no `precommit` target — do not run make there.
</constraints>

<verification>
Run each of these and confirm the stated result.

1. `ROOTDIR=/workspace make precommit` — exits 0.
2. `! grep -q 'envparse.KeyValuePairs(a\.' main.go` — exits 0.
3. `! grep -q 'envparse.KeyValuePairs(a\.' cmd/run-task/main.go` — exits 0.
4. `! grep -qE '^\s*(ClaudeEnvRaw|EnvContextRaw)\s+string\b' main.go` — exits 0 (neither field is still a plain `string`).
5. `! grep -qE '^\s*(ClaudeEnvRaw|EnvContextRaw)\s+string\b' cmd/run-task/main.go` — exits 0.
6. `! grep -qE 'display:"(length|hidden)".*(CLAUDE_ENV|ENV_CONTEXT)|(CLAUDE_ENV|ENV_CONTEXT).*display:"(length|hidden)"' main.go cmd/run-task/main.go` — exits 0.
7. `grep -n '^## Unreleased' CHANGELOG.md` — prints exactly one line.
8. `grep -q 'INNOCUOUS_NAME' main_internal_test.go` — exits 0 (the requirement-5 regression spec exists).
9. `grep -q 'os.Setenv("CLAUDE_ENV"' main_internal_test.go` — exits 0 (the parser contract spec exists).
10. `test -f pkg/envbag/envbag_suite_test.go` — exits 0.
</verification>
