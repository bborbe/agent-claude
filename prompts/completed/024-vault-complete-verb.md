---
status: completed
summary: Added agent/scripts/vault-complete.sh plus its fixture-driven test, documented the verb in § Vault and the allowlists, and recorded a changelog entry; ROOTDIR=/workspace make precommit exits 0.
execution_id: agent-claude-exec-024-vault-complete-verb
dark-factory-version: v0.198.7
created: "2026-10-19T23:55:00Z"
queued: "2026-10-19T22:04:59Z"
started: "2026-10-19T22:08:01Z"
completed: "2026-10-19T22:13:39Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. WHAT THIS CLOSES. A cluster worker can already write vault files through the
   `git-rest` service, but it has no way to move a task row's `status` to
   `completed`. `vault-set-section.sh` splices `#`-heading sections and does not
   touch YAML frontmatter, so flipping `status:` needs a new verb, not a new
   transport. This prompt adds that one verb.

2. WHY THE SCRIPT AND NOT `vault-cli`. The alternative route was to bake
   `vault-cli` into the image, which would put a second git writer on the vault
   repository. `git-rest`'s own rule is a single git writer per repo. The
   pod-side verb reuses the route that is already proven at both ends, keeps
   `git-rest` the only writer, and adds no image weight and no vault credential
   to the pod. The cost is accepted deliberately: this verb flips `status` only,
   it is not the full `vault-cli` surface.

3. THE FRONTMATTER BOUNDARY IS THE WHOLE RISK. A naive `sed -i 's/^status:.*/.../'`
   would also rewrite a `status:` line inside the document body, and a naive
   whole-file rewrite is the exact failure that destroyed a task file on
   2026-10-11. The function is therefore scoped to the region between the first
   line and the closing `---`, refuses every ambiguous shape loudly, and is
   tested against fixtures with no server.

4. NO DOCKERFILE EDIT. `COPY agent/ /agent/` already ships the whole tree, so a
   new file under `agent/scripts/` reaches the pod with no image change.

5. THE REAL PROOF IS OPERATOR-SIDE. A live pod run against
   `vault-obsidian-personal:9090` is the only end-to-end evidence, and this
   container cannot reach that host. Every check below is local and
   fixture-driven; the live run is named in `<constraints>` as out of scope.

6. HIDE-GIT. If the daemon is launched with `--set hideGit=true` (the operator's
   invocation does), `.git` is masked and every bare `git` command fails. No `git`
   command appears in `<verification>`. `ROOTDIR=/workspace` is passed on every
   `make` call, as the dark-factory guide requires under a masked `.git`; in this
   repo `ROOTDIR` is only read by `trivy`'s ignorefile fallback, so the call would
   work without it, but passing it is the documented container-safe form.
-->

# A vault-completion verb for cluster workers

<summary>
- A worker can now mark a vault task complete without hand-writing a request.
- The verb changes exactly one line — the task's `status` in its YAML frontmatter — and leaves every other byte of the file alone.
- A body line that merely looks like `status:` is never touched, because the change is scoped to the frontmatter block.
- Anything ambiguous stops loudly instead of guessing: no frontmatter, no `status` key, a duplicate `status` key, or an unclosed frontmatter block all fail with a message naming the file.
- Running it twice produces the same file, so a retried turn cannot corrupt a row.
- The structural behaviour is proven locally against fixtures with no server.
- The agent's guardrails name the new verb beside the three existing helpers, so it is discoverable rather than invented.
- No Dockerfile change, no new package, no new binary and no new credential.
- A changelog entry records the capability, so the release that follows carries it.
</summary>

<objective>
Give the pod a single narrow verb that moves a vault task row to `status: completed` on the vault's remote: add `agent/scripts/vault-complete.sh <repo-relative-path>`, which GETs the file, replaces exactly the one `status:` line inside its YAML frontmatter, and POSTs the whole file back so the `git-rest` service commits and pushes it — so a cluster worker can close its own row instead of only being able to write files.
</objective>

<context>
Read `docs/dod.md` first — it is the Definition of Done you are graded against and this repository's declared `validationPrompt`. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `autoRelease: false`, `autoGeneratePrompts: true`.

This is a standalone prompt: there is no spec file to read. Everything you need is below.

Read these files before editing:

- `agent/.claude/CLAUDE.md` — read the WHOLE file. `## Vault` is the section you extend; the bullet list under **Use the helper scripts** is where the new verb is named. `## Forbidden`, `## Tools` and `## Source repositories` are neighbours you must not change.
- `agent/scripts/vault-set-section.sh` — the exemplar for this change. Copy its shape: `set -e -u -o pipefail`, the script-scope temp variables with an `EXIT` trap (a trap reading a `local` that has already gone raises `unbound variable` under `set -u` and turns a successful write into exit 1), a pure function that emits to stdout, a `main` that does the HTTP, and the `if [ "${BASH_SOURCE[0]}" = "$0" ]; then main "$@"; fi` guard that lets the test source the script without running it.
- `agent/scripts/vault-set-section_test.sh` — the exemplar for the test: fixture-driven, no server, one `PASS`/`FAIL` line per case, non-zero exit if any assertion fails.
- `agent/scripts/vault-read.sh` and `agent/scripts/vault-write.sh` — the GET and POST halves must make the same requests these two files make: the same URL, the same two gateway headers, `-sf --max-time 30`, `--data-binary`.
- `CHANGELOG.md` — line 1 is `# Changelog`; the first `## ` heading is `## v0.21.1`. There is **no** `## Unreleased` section today (`grep -c '^## Unreleased' CHANGELOG.md` prints `0`).

Facts verified against the working tree, which your change must honour:

- `agent/scripts/` already exists and holds `vault-read.sh`, `vault-write.sh`, `vault-set-section.sh` and `vault-set-section_test.sh`. You add two files to it; you edit none of those four. `Makefile.precommit` sets `SHELL_SCRIPTS = $(wildcard agent/scripts/*_test.sh)` and `precommit` depends on `shell-test`, so every `agent/scripts/*_test.sh` — including the new test — runs on `make precommit`.
- The runtime stage already does `COPY agent/ /agent/`, and the agent's cwd is `/agent`, so `scripts/vault-complete.sh` resolves in the pod with **no Dockerfile change**.
- The vault host is `http://vault-obsidian-personal:9090`. Every `/api/v1/*` request needs two headers: `X-Gateway-Initator` (free-form caller identity — **the spelling is deliberate, `Initator` not `Initiator`; do not "correct" it**) and `X-Gateway-Secret: $GATEWAY_SECRET`.
- `GATEWAY_SECRET` is already plumbed into the pod and forwarded into the Claude child environment (`claudeEnv["GATEWAY_SECRET"] = a.GatewaySecret` in `main.go`), so it is present in the shell environment of a `Bash` tool call.
- `curl` is already in the image. Nothing here needs installing.
- A vault task file's frontmatter is the block between line 1 (`---`) and the next line that is exactly `---`. `status` is one of its keys; a task row carries exactly one. The target value `completed` is the vault's canonical terminal status (the status set is `next`, `in_progress`, `backlog`, `completed`, `hold`, `aborted`) — use that literal, not `done` or `closed`.

### What is out of scope

Restated because the executing agent has no memory of any spec: no change to `main.go`, `go.mod`, `go.sum`, `pkg/`, `cmd/` or any `.go` file; no Dockerfile change; no new package, binary, symlink or alias; no change to the gateway service (it lives in another repository); no change to `## Forbidden`, `## Source repositories`, `## Tools`, `## Output`, `## Scope` or `## Data` in `agent/.claude/CLAUDE.md`; no Makefile target; no new credential; no general-purpose "set any status" verb — this one sets `completed` and nothing else.

Out of scope but load-bearing: a task shape whose deployment constrains `Bash` to the named helpers needs `Bash(scripts/vault-complete.sh:*)` in its `ALLOWED_TOOLS`, or the new verb is documented but unreachable. That field lives in the deployment (another repo) — name the dependency in the completion report, do not attempt to change it from here. The pattern is documented in `docs/creating-claude-agents.md`.
</context>

<requirements>
1. **Add `agent/scripts/vault-complete.sh <repo-relative-path>`.** Created executable (`chmod 0755 agent/scripts/vault-complete.sh`) — `COPY agent/ /agent/` preserves the mode git records, and § Vault invokes it as a program, so a 0644 file is unreachable in the pod. Same header shape as the sibling helpers: a `#!/bin/bash` shebang and a comment header naming purpose and usage.

   a. **A pure, sourceable function `set_status_completed <file>` that emits the result to stdout.** It must:
      - treat the frontmatter as the region from line 1 through the next line whose content is exactly `---`;
      - **fail loudly, non-zero, with a stderr message naming the file**, when line 1 is not exactly `---`, when there is no closing `---` line, when the frontmatter contains no line matching `^status:`, or when it contains two or more such lines — do not guess in any of those cases;
      - replace exactly that one line with `status: completed` and preserve **every other byte**, including other frontmatter keys, every body line, and the file's trailing-newline state. A line in the body that begins with `status:` must come through untouched;
      - be idempotent: a file already carrying `status: completed` produces byte-identical output.

   b. **A guard so the function is sourceable without executing** — `if [ "${BASH_SOURCE[0]}" = "$0" ]; then main "$@"; fi`.

   c. **The main path**, for direct invocation: `set -e -u -o pipefail`; require the secret loudly and first with `: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"`; validate exactly 1 argument (usage line to stderr, non-zero exit otherwise); compute `path="${1// /%20}"`; GET the file with `curl -sf --max-time 30` and the two gateway headers into a `mktemp` file; run `set_status_completed` on it into a second `mktemp` file; POST that file with `-X POST`, **the same two gateway headers**, `-H 'Content-Type: application/octet-stream'` and `--data-binary "@<temp>"` to `http://vault-obsidian-personal:9090/api/v1/files/<path>`. Every `/api/v1/*` request needs both gateway headers — a POST without them is refused before it touches the vault. Define the two header strings once as `main`-local variables (`initiator_header="X-Gateway-Initator: agent-claude"`, `secret_header="X-Gateway-Secret: $GATEWAY_SECRET"`) and pass them to both requests, exactly as `vault-set-section.sh` does — `<verification>` items 3 and 4 count each literal exactly once, so a second inline copy fails them. Clean both temp files up on every exit path — hoist them to script scope, as `vault-set-section.sh` does, so the `EXIT` trap can still see them.

   `--data-binary` is mandatory; `--data` and `-d` strip newlines and would corrupt the file.

2. **Add `agent/scripts/vault-complete_test.sh`** — fixture-driven, **no server**. It resolves its own directory (`here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"`), sources `${here}/vault-complete.sh`, resets `set +e +u +o pipefail` after sourcing, builds fixtures in a temp directory it creates and cleans up, and asserts at least these seven cases. Each assertion prints a `PASS`/`FAIL` line naming the case, and the script exits non-zero if any fails:

   - **replace:** a vault-shaped file whose frontmatter carries `status: in_progress` alongside other keys, and whose body carries a `# Progress` section including a line beginning `status:`; after `set_status_completed`, the frontmatter reads `status: completed`, the other keys are unchanged, the body's `status:` line is unchanged, and the trailing-newline state is unchanged.
   - **idempotent:** a file already at `status: completed` produces output identical to its input.
   - **no frontmatter:** a file starting `# Title` returns non-zero and the stderr message names the file.
   - **no status key:** frontmatter present but without a `status:` line returns non-zero.
   - **duplicate status:** two `status:` lines inside the frontmatter return non-zero.
   - **unclosed frontmatter:** `---` on line 1 with no closing `---` returns non-zero.
   - **process:** with a stub `curl` on `PATH` (the harness `vault-set-section_test.sh` already uses) that records its argv, a direct `bash agent/scripts/vault-complete.sh <path>` GETs the file, POSTs the modified bytes with `--data-binary`, sends both gateway headers (assert the header **names** and the initiator value; do NOT write a literal `X-Gateway-Secret: <value>` into the test file — verification item 22 rejects any literal secret header), and leaves no `mktemp` files behind on the success path; with `GATEWAY_SECRET` unset it exits non-zero with the `:?` message; with a failing POST it exits non-zero and still leaves no temp files.

3. **Name the verb in `agent/.claude/CLAUDE.md` § Vault.** Under **Use the helper scripts**, add one bullet beside the existing three:
   `- \`scripts/vault-complete.sh <repo-relative-path>\` — GETs the file, sets the task's \`status\` to \`completed\` in its YAML frontmatter, and POSTs the whole file back, leaving every other byte untouched.`
   Also add one clause to the existing "For a change confined to one section…" bullet stating that `vault-complete.sh` is the tool for a status flip, and that a section helper cannot do it because it splices `#`-heading sections rather than YAML frontmatter. Also change the list's lead-in from `These three are how the read-modify-write above is done:` to `These four are how the read-modify-write above is done:` — the new bullet makes the count wrong. Change nothing else in the section, and nothing outside it.

4. **Add the `## Unreleased` changelog entry.** `CHANGELOG.md` has no `## Unreleased` section today; insert one directly above `## v0.21.1`, holding **exactly one** bullet prefixed `feat:`. The bullet must state that a cluster worker could write vault files but had no verb to move a task row's `status` to `completed`; that `agent/scripts/vault-complete.sh` now GETs the file, sets the frontmatter `status` to `completed` and POSTs the whole file back so the service commits and pushes; that the change is scoped to the frontmatter block and fails loudly on a missing, duplicate or unclosed shape; and that its fixture-driven test is run by `make precommit` through the `shell-test` target's `agent/scripts/*_test.sh` glob. Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes. The bullet MUST contain the exact token `vault-complete.sh` and MUST NOT contain a credential value of any kind. Do not move, delete or reorder the preamble, and do not edit any released section.

5. **Keep the README's allowlist row in step — and change nothing else.** In `README.md`, the `Vault I/O via scripts` row lists the helper allowlist; add `Bash(scripts/vault-complete.sh:*)` to it so the documented allowlist matches the new helper. That is the **only** permitted README edit — do not touch the row's other entries, and do not "fix" the stale `vault-list.sh` reference in it. `docs/creating-claude-agents.md` carries the same two lists; add `Bash(scripts/vault-complete.sh:*)` to its `Vault I/O via git-rest` allowlist row (line 70) and `vault-complete.sh` to its helper list (line 90), and change nothing else in that file. Do NOT edit `Dockerfile`, `main.go`, any other `.go` file, `go.mod`, `go.sum`, `Makefile*`, `.dark-factory.yaml`, the repo-root `scripts/` directory, or the four existing files under `agent/scripts/`. Do NOT add a `Makefile.precommit` target.

6. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement against the change you actually made. In particular confirm (i) the only files you touched are `agent/scripts/vault-complete.sh`, `agent/scripts/vault-complete_test.sh`, `agent/.claude/CLAUDE.md`, `CHANGELOG.md`, and the two allowlist/helper-list edits in `README.md` and `docs/creating-claude-agents.md`; (ii) both new scripts are executable and pass `bash -n`; (iii) the test prints a `PASS` line for all seven cases; (iv) `vault-complete.sh` requires `GATEWAY_SECRET` with `:?`, carries both headers with the deliberate `Initator` spelling, and uses `--data-binary`; (v) `## Vault` still has exactly one heading and still names the three existing helpers; (vi) `## Unreleased` sits above `## v0.21.1` with exactly one bullet.
</requirements>

<constraints>
- **Do NOT commit — dark-factory handles git.** Do not run any `git` command: the container is started with `.git` masked (`hideGit=true`), so every `git` invocation fails — and a failed command in `<verification>` is a false-positive pass, because the daemon's executor does not check verification exit codes.
- **Do NOT run `git` against the vault**, and do not add any instruction telling the agent to. The `git-rest` service owns the vault's git; the pod never touches it.
- **Never print, log, write to a file, or place in argv the value of `GATEWAY_SECRET`.** It goes into one place only: the `X-Gateway-Secret` header of a request to `http://vault-obsidian-personal:9090`. A script that echoes the secret for debugging is a defect, not a convenience.
- **The header name is `X-Gateway-Initator`** — spelled `Initator`, not `Initiator`. The misspelling is deliberate and the service requires it. Do not "correct" it anywhere.
- **`--data-binary`, never `--data` or `-d`.** `--data` strips newlines and corrupts every vault file.
- **`agent/scripts/` is the placement.** Not repo-root `scripts/` (which holds image-level Python tools copied individually to `/usr/local/bin/`) and not `/usr/local/bin/`. The runtime stage's `COPY agent/ /agent/` is what puts the files in the pod, which is why no `Dockerfile` edit is required. Do not add one.
- **The gateway host is reachable from a pod, not from this container.** No live GET or POST may appear in `<verification>` and none may be attempted. Every check is local and fixture-driven. The real end-to-end proof — a live pod run through the gateway, then a re-read showing `status: completed` on the vault's remote — is **operator-side and deliberately out of scope**; do not attempt it and do not claim it.
- **The frontmatter boundary is load-bearing.** A change that rewrites any body line, or that treats a body `status:` line as the target, is a defect even if the status still ends up correct.
- **No new package, binary, symlink, alias, or credential.** `curl` is already in the image.
- **No Go code.** This change is two shell files, one markdown bullet list and one changelog bullet.
- **Existing tests must still pass.** `ROOTDIR=/workspace make precommit` must exit 0.
- **Do NOT edit `CHANGELOG.md` sections other than the new `## Unreleased` section**, and add exactly one bullet to it.
- **Do not mention the counted tokens in script comments.** `<verification>` requires exact counts of `1` for `X-Gateway-Initator: agent-claude`, `X-Gateway-Secret: $GATEWAY_SECRET`, `--data-binary` and the `:?` requirement, and item 6 requires a `--data` flag (not the `--data-binary` substring) and a bare `-d` to appear nowhere in the script; a comment repeating any of these — even to forbid it — fails that step. A comment header naming purpose and usage does not need those literals.
- **The new test IS wired into `make precommit`.** `Makefile.precommit` sets `SHELL_SCRIPTS = $(wildcard agent/scripts/*_test.sh)` (line 21) and the `precommit` target depends on `shell-test` (line 5), which runs each matched file with `bash`. `agent/scripts/vault-complete_test.sh` therefore runs on every `make precommit`; it must be self-contained and must not require `GATEWAY_SECRET` (the `:?` check lives inside `main`, so sourcing the script is safe).
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`. No `git` command appears below, and nothing below reaches `vault-obsidian-personal:9090`.

A note on reading the results: the "must be absent" checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `bash -n agent/scripts/vault-complete.sh && bash -n agent/scripts/vault-complete_test.sh` — exits 0 (both scripts are syntactically valid bash).
2. `bash agent/scripts/vault-complete_test.sh` — exits 0 and prints a `PASS` line for each of the `replace`, `idempotent`, `no frontmatter`, `no status key`, `duplicate status`, `unclosed frontmatter` and `process` cases. This is the structural proof, run against fixtures with no server.
3. `grep -cF 'X-Gateway-Initator: agent-claude' agent/scripts/vault-complete.sh` — prints `1` (the helper sends the initiator header, with the deliberate spelling).
4. `grep -cF 'X-Gateway-Secret: $GATEWAY_SECRET' agent/scripts/vault-complete.sh` — prints `1` (the helper sends the secret from the environment).
5. `grep -c -- '--data-binary' agent/scripts/vault-complete.sh` — prints `1` (the POST sends the file's bytes verbatim).
6. `! grep -qE -- '--data([^-]|$)|(^|[[:space:]])-d([[:space:]]|$)' agent/scripts/vault-complete.sh` — exits 0 (the helper uses neither `--data` nor a bare `-d`, which would strip newlines and corrupt the file).
7. `grep -cF ': "${GATEWAY_SECRET:?' agent/scripts/vault-complete.sh` — prints `1` (the secret is required loudly).
8. `out=$(GATEWAY_SECRET=SENTINEL-DO-NOT-ECHO bash agent/scripts/vault-complete.sh 2>&1); printf '%s' "$out" | grep -q 'SENTINEL-DO-NOT-ECHO' && echo LEAK || echo clean` — prints `clean` (with no arguments the helper takes its usage path and does not echo the secret; the sentinel is a throwaway literal, not a credential).
9. `! grep -qE 'X-Gateway-Initiator|Initiator:' agent/scripts/vault-complete.sh agent/.claude/CLAUDE.md` — exits 0. No script uses the corrected header spelling. `§ Vault` legitimately contains the bare word `Initiator` inside its "the spelling is deliberate" note; that note has no colon after the word and no `X-Gateway-` prefix, so neither alternative in this pattern matches it — the note must stay, and this check must stay green with it present.
10. `test -x agent/scripts/vault-complete.sh && test -x agent/scripts/vault-complete_test.sh && echo executable` — prints `executable`.
11. `grep -c '^## Vault$' agent/.claude/CLAUDE.md` — prints `1` (the section was extended in place, not duplicated).
12. `awk '/^## Vault/{f=1;next} /^## /{f=0} f' agent/.claude/CLAUDE.md | grep -c 'vault-complete.sh'` — prints at least `1` (the new verb is named in § Vault).
13. `awk '/^## Vault/{f=1;next} /^## /{f=0} f' agent/.claude/CLAUDE.md | grep -c 'vault-set-section.sh'` — prints at least `1` (the existing helpers are still named).
14. `grep -cF '**One named exception: `vault-obsidian-personal:9090`**' agent/.claude/CLAUDE.md` — prints `1` (the `## Forbidden` network exception is unchanged).
15. `grep -cE '^## Source repositories$' agent/.claude/CLAUDE.md` — prints `1` (the neighbouring section is untouched).
16. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
17. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
18. `grep -c 'vault-complete.sh' CHANGELOG.md` — prints `1` (the changelog bullet).
19. `awk '/^## /{sec=$0} /vault-complete.sh/{print "sits under: " sec}' CHANGELOG.md` — prints `sits under: ## Unreleased`.
20. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (exactly one bullet).
21. `grep -c '^## v0.21.1' CHANGELOG.md` — prints `1` (the previously-first released heading is still present, directly below the new section).
22. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|X-Gateway-Secret: [^$]' agent/.claude/CLAUDE.md agent/scripts/vault-complete.sh agent/scripts/vault-complete_test.sh CHANGELOG.md` — exits 0 (no credential value and no literal secret in any file this change touches).
23. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command and the exit code the completion report carries.
</verification>

<success_criteria>
- `agent/scripts/vault-complete.sh` exists, is executable, passes `bash -n`, requires `GATEWAY_SECRET` with `:?`, carries `X-Gateway-Initator: agent-claude` (that spelling) and `X-Gateway-Secret: $GATEWAY_SECRET`, and POSTs with `--data-binary`.
- Its `set_status_completed` function emits to stdout, replaces exactly one `status:` line inside the YAML frontmatter with `status: completed`, preserves every other byte including body lines that begin `status:` and the file's trailing-newline state, is idempotent, and fails loudly and non-zero on a missing frontmatter, an unclosed frontmatter, a missing `status` key and a duplicate `status` key.
- Its GET and POST halves make the same requests `vault-read.sh` and `vault-write.sh` make.
- `agent/scripts/vault-complete_test.sh` passes all seven fixture cases with no server.
- `agent/.claude/CLAUDE.md` § Vault names `scripts/vault-complete.sh` with its argument shape, states that it is the tool for a status flip, and leaves the three existing helpers and every other section unchanged.
- `CHANGELOG.md` gains a `## Unreleased` section with exactly one `feat:` bullet containing `vault-complete.sh`, directly above `## v0.21.1`, and no released section is edited.
- No credential value and no literal secret appears in any file this change touches.
- Only `agent/scripts/vault-complete.sh`, `agent/scripts/vault-complete_test.sh`, `agent/.claude/CLAUDE.md`, `CHANGELOG.md`, and the allowlist/helper-list edits in `README.md` and `docs/creating-claude-agents.md` are modified.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
