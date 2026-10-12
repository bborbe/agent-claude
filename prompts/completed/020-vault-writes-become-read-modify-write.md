---
status: completed
summary: 'Made vault writes read-modify-write by rewriting agent/.claude/CLAUDE.md § Vault and adding vault-read.sh, vault-write.sh, vault-set-section.sh and a fixture-driven splice test under agent/scripts/, with a fix: CHANGELOG entry.'
execution_id: agent-claude-vault-write-exec-020-vault-writes-become-read-modify-write
dark-factory-version: v0.196.0
created: "2026-10-12T14:52:00Z"
queued: "2026-10-12T15:16:31Z"
started: "2026-10-12T15:22:45Z"
completed: "2026-10-12T15:38:48Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. WHAT SHIPPED WRONG. A cluster pod wrote a task file into an Obsidian vault through the
   git-rest gateway (`POST /api/v1/files/<path>`, HTTP 200) and destroyed it: three sections
   duplicated, the whole `# Progress` section replaced, and the file left truncated
   mid-sentence with no trailing newline (commit `c3c1517761`). The diagnosis is
   code- and git-verified: the gateway writes the POST body verbatim (`os.WriteFile` of the
   exact bytes received) and the corrupting commit carries the POST message
   `git-rest: update <path>`, so the merge resolver is exonerated and the body the pod sent
   WAS the corrupted document — the file's own content re-spliced, a window replayed and cut
   mid-line at both ends. Nothing on the pod side made a malformed body impossible: the
   agent's only vault-write instruction was "POST the content as the body", with no
   read-modify-write protocol and no script, so the body was model-generated text inside a
   tool call. This prompt closes that hole at the instruction and at the mechanism.

2. WHY `agent/scripts/` AND NOT REPO-ROOT `scripts/`. The repo-root `scripts/` holds
   image-level Python tools that `Dockerfile` copies INDIVIDUALLY to `/usr/local/bin/`
   (`COPY scripts/pod-attention.py scripts/answered-attribution.py /usr/local/bin/` and
   `COPY scripts/git_credential_github_app.py /usr/local/bin/git-credential-github-app`).
   The proven precedent (`trading/agent/trade-analysis/agent/scripts/vault-write.sh`) puts
   agent-facing scripts under `agent/scripts/`, and this repo's runtime stage already does
   `COPY agent/ /agent/` — so `agent/scripts/` reaches the pod with **no Dockerfile change**.
   The agent's cwd is `/agent`, so `scripts/vault-write.sh` resolves, which is exactly what
   `agent/.claude/CLAUDE.md` § Tools ("Scripts in `scripts/` are your API — use them, do not
   reimplement") already claims. This is the one thing about the change a reviewer is most
   likely to want to relocate; do not — it is load-bearing.

3. NO DOCKERFILE EDIT — deliberately. `COPY agent/ /agent/` already ships the whole tree, so
   adding files under `agent/scripts/` is the only placement that reaches the pod with zero
   image change. A `COPY` into `/usr/local/bin/` would also have worked but would have
   split the scripts from the guardrail that names them, and would have needed a new
   Dockerfile directive.

4. THE SPLICE IS TESTABLE WITHOUT A SERVER, ON PURPOSE. `vault-set-section.sh` keeps its
   `splice_section` function pure (file, heading, content-file → stdout) so the test can
   exercise it against fixtures with no gateway. The sibling `vault-set-section_test.sh` is
   the only container-executable proof of the structural fix; it is invoked directly by
   `<verification>` (via `bash`), not wired into `make precommit` — this repo's precommit
   runs `python-check`/`python-test` over `scripts/*.py` and nothing over shell, and the user
   did not ask for a Makefile target.

5. THE REAL PROOF IS OPERATOR-SIDE. A live pod write against
   `vault-obsidian-personal:9090` is the only end-to-end evidence that the corruption cannot
   recur, and this container cannot reach that host. Every check below is a local,
   fixture-driven assertion; the live write is named in `<constraints>` as deliberately out
   of scope.

6. HIDE-GIT. `.dark-factory.yaml` sets `workflow: direct`, and this repository's daemon runs
   with `hideGit=true` (documented in `prompts/completed/015-…` and `019-…`), so `.git` is
   masked and every bare `git` command fails. No `git` command appears in `<verification>`,
   and `ROOTDIR=/workspace` is passed on every `make` call — masking `.git` also empties
   `ROOTDIR`, and `make` then dies before any target runs.
-->

# Vault writes become read-modify-write

<summary>
- A vault write is now defined as read-modify-write: fetch the file, change a local copy, send the whole file back.
- The agent's guardrails say plainly that a write replaces the entire file, so anything left out of the body is destroyed, and that the body must never be composed from memory.
- Two ready-made helpers ship inside the pod so the agent never hand-writes a vault request: one that posts a whole local file, and one that updates a single named section and leaves every other byte alone.
- Updating one section no longer risks the rest of the file, which is the exact failure that corrupted a task file in production.
- A section update that would be ambiguous — the same heading appearing twice — stops loudly instead of guessing.
- The agent is told which helper to use and when, so a whole-file write is a deliberate choice rather than the default path.
- The helpers carry the gateway headers and refuse to run without the shared secret, so a misconfigured pod fails with a clear message instead of a confusing authorisation error.
- The structural fix is proven locally against fixtures with no server: replace, append, and duplicate-heading cases all have an assertion.
- No Dockerfile change is needed, and no new package, binary or credential is introduced.
- A changelog entry records the fix, so the release that follows carries it.
</summary>

<objective>
Make a vault write a read-modify-write by default, and give the pod the mechanism to do it: rewrite `agent/.claude/CLAUDE.md` § Vault so a write is explicitly "GET first, modify a local copy, POST the WHOLE file back" with the replacement semantics stated outright, and add three shell helpers under `agent/scripts/` — `vault-read.sh` (GET), `vault-write.sh` (POST a whole local file) and `vault-set-section.sh` (GET, replace or append exactly one named section, POST the result) — so that a pod updating a single section of a vault file can no longer destroy the rest of it by authoring a body from memory.
</objective>

<context>
Read `docs/dod.md` first — it is the Definition of Done you are graded against, and this repository's declared `validationPrompt`. Read `.dark-factory.yaml` — it is the authority that travels with the repo (`workflow: direct`, `pr: false`, `autoRelease: false`, `autoGeneratePrompts: true`). This repository has no root `CLAUDE.md` (it is in `.gitignore`), so do not look for one.

This is a standalone prompt: there is no spec file to read. Everything you need is below.

Read these files before editing:

- `agent/.claude/CLAUDE.md` — read the WHOLE file. The `## Vault` section is the block you rewrite; the `## Forbidden`, `## Tools` and `## Source repositories` sections are the neighbours whose wording you must preserve.
- `agent/.claude/CLAUDE.md` § Vault, the "Write" bullet — the single instruction this change replaces. It currently reads: `**Write** a file: `POST /api/v1/files/<path>` with `Content-Type: application/octet-stream` and the content as the body; nested directories are created for you`. Nothing in the file says what a write *replaces*, and nothing gives a read-modify-write protocol. That gap is the defect.
- `CHANGELOG.md` — read the preamble and the top of the file. Line 1 is `# Changelog`; the first `## ` heading is `## v0.19.0` at line 9. There is **no** `## Unreleased` section today (`grep -c '^## Unreleased' CHANGELOG.md` prints `0`).
- `Dockerfile` — confirm for yourself that the runtime stage already copies the whole `agent/` tree. The line is `COPY agent/ /agent/`. You will **not** edit this file; the line is the reason you do not have to.

Facts verified against the working tree, which your change must preserve or honour:

- `agent/` today contains exactly one file: `agent/.claude/CLAUDE.md`. There is **no** `agent/scripts/` directory yet — you create it.
- The runtime stage is the final `FROM alpine`, and it does `COPY agent/ /agent/`. The agent's working directory is `/agent`, so a file at `agent/scripts/vault-write.sh` in the repo is reachable inside the pod as `scripts/vault-write.sh`, relative to the agent's cwd.
- `agent/.claude/CLAUDE.md` § Tools already says: "Scripts in `scripts/` are your API — use them, do not reimplement". The scripts you add are what that sentence refers to.
- The vault host is `http://vault-obsidian-personal:9090` — the one internal host `## Forbidden` permits. Every `/api/v1/*` request needs two headers: `X-Gateway-Initator` (free-form caller identity — **the spelling is deliberate, `Initator` not `Initiator`; do not "correct" it anywhere**) and `X-Gateway-Secret: $GATEWAY_SECRET`. The probes `/healthz`, `/readiness` and `/metrics` take **no** headers by design.
- `GATEWAY_SECRET` is already plumbed into the pod: it is declared in `main.go` (`GatewaySecret string \`required:"false" arg:"gateway-secret" env:"GATEWAY_SECRET" …\``) and forwarded into the Claude child environment (`claudeEnv["GATEWAY_SECRET"] = a.GatewaySecret`). It is therefore present in the shell environment of a `Bash` tool call, which is where these scripts read it from.
- The repo-root `scripts/` directory holds **Python** tools only (`pod-attention.py`, `answered-attribution.py`, `git_credential_github_app.py`, `git_credential_github_app_test.py`). `make precommit`'s `python-check`/`python-test` targets glob `scripts/*.py` — they do not touch `agent/scripts/*.sh`. Do not put shell scripts in repo-root `scripts/`.

### Why `agent/scripts/` and not repo-root `scripts/` — the reason no Dockerfile edit is required

The repo-root `scripts/` holds **image-level** Python tools that the `Dockerfile` copies **individually** to `/usr/local/bin/`:

```dockerfile
COPY scripts/pod-attention.py scripts/answered-attribution.py /usr/local/bin/
COPY scripts/git_credential_github_app.py /usr/local/bin/git-credential-github-app
```

A new file there would reach the image **only** if someone added another `COPY` line. By contrast the runtime stage already does `COPY agent/ /agent/`, so anything under `agent/` is in the pod with **no Dockerfile change at all** — and `agent/scripts/` is the same placement the proven precedent uses (see below). That is why these scripts go under `agent/scripts/` and why this prompt edits no `Dockerfile`.

### The shape to inline (there is no in-repo exemplar)

This repository has **no** shell-script precedent — `rg -l 'set -e' --glob '*.sh' .` returns nothing, and `agent/` holds one markdown file. The proven precedent lives in a **sibling repository** (`trading/agent/trade-analysis/agent/scripts/vault-write.sh`), which is **not visible inside this container**, so its shape is reproduced here rather than linked. Reproduce this shape; do not invent your own:

```bash
set -e -u -o pipefail

path="${1// /%20}"                       # spaces are the common reserved char in vault paths

exec curl -sf --max-time 30 \
  -X POST \
  -H 'Content-Type: application/octet-stream' \
  --data-binary "@${2}" \
  "${VAULT_BASE}/api/v1/files/${path}"
```

Three details in that shape are load-bearing and must survive into your version:

1. `set -e -u -o pipefail` at the top — `-u` in particular, so an unset variable fails loudly instead of silently producing an empty header.
2. `path="${1// /%20}"` — bash parameter expansion replacing spaces with `%20`. URL-encode spaces; other reserved characters are rare in vault paths and are deliberately not encoded.
3. `--data-binary "@${2}"`, **not** `--data "@${2}"` and **not** `-d`. `--data` strips newlines and mangles binary; `--data-binary` sends the file's bytes verbatim. A vault file is text with significant newlines, so the wrong flag silently corrupts every write.

Coding guides (in-container paths — read the ones your change touches):

- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement, the required conventional prefix, the frozen-preamble rule, and the anti-patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
- `/home/node/.claude/plugins/marketplaces/coding/docs/documentation-guide.md` — the prose style for the guardrail section you rewrite (imperative, reason-first).

### What is out of scope

Restated here because the executing agent has no memory of any spec: no change to `main.go`, `go.mod`, `go.sum`, `pkg/`, `cmd/` or any `.go` file; no Dockerfile change; no new package, binary, symlink or alias; no change to the gateway service itself (it lives in another repository — this prompt only changes what the pod *sends*); no change to `## Forbidden`, `## Source repositories`, `## Tools`, `## Output`, `## Scope` or `## Data` in `agent/.claude/CLAUDE.md`; no Makefile target; no new credential, no change to how `GATEWAY_SECRET` is plumbed.
</context>

<requirements>
1. **Rewrite the `## Vault` section of `agent/.claude/CLAUDE.md` so a write is a read-modify-write.** The section is currently the block beginning `## Vault` and ending immediately before `## Source repositories`. Rewrite it in place — do not move it, do not add a second `## Vault` heading, and do not renumber or retitle it. The rewritten section must:

   a. **State the replacement semantics outright, as its own prominent sentence.** The exact wording must include the sentence: `A write REPLACES the whole file. Anything omitted from the body is destroyed.` Add directly after it, in the same paragraph, that the body must therefore **never** be authored from memory — it must be the bytes of a local file that was read from, or derived from, the file's current content. Give the reason: a body written from memory silently drops every part of the file the model did not think to reproduce, and the service has no way to tell a deliberate truncation from a mistake.

   b. **Spell out the read-modify-write protocol as a numbered sequence**, in this order:
      1. **GET** the file first — `GET /api/v1/files/<path>` — and keep the result as a local file.
      2. **Modify the local copy** — never the response body in flight, and never a reconstruction from memory.
      3. **POST the WHOLE file back** — `POST /api/v1/files/<path>`, `Content-Type: application/octet-stream`, the entire local file as the body.
      State that step 3 sends the complete document, so the local file must be the full current content plus your change — not the change alone.

   c. **Name the helper scripts as the mechanism, with their argument shapes.** State that the scripts are how you do this, and give each one's call form:
      - `scripts/vault-read.sh <repo-relative-path>` — GETs a file to stdout.
      - `scripts/vault-write.sh <repo-relative-path> <local-file>` — POSTs a whole local file.
      - `scripts/vault-set-section.sh <repo-relative-path> <heading> <content-file>` — GETs the file, replaces or appends exactly the one named section, and POSTs the result, leaving every other byte untouched.
      State that for a change confined to one section, `vault-set-section.sh` is the correct tool and `vault-write.sh` is for whole-file writes only. State that a script in `scripts/` is the API (§ Tools already says so) and that hand-rolled `curl` calls are what this section exists to replace.

   d. **Preserve, byte-for-byte, these existing elements of the section** (the rest of the section's prose may be tightened or reordered, but these must survive unchanged in meaning and, where a spelling is called out, in spelling):
      - The `X-Gateway-Initator` header name — spelled `Initator`, with the existing note that the spelling is deliberate and must not be "corrected".
      - The `X-Gateway-Secret: $GATEWAY_SECRET` header and the rule that it is never printed, logged, written to a file, or sent to any host but this one.
      - The two-refusals paragraph: `500 header 'X-Gateway-Initator' missing` and `401 secret in header 'X-Gateway-Secret' is invalid`, including the note that a `401` is the likeliest outcome when `GATEWAY_SECRET` never reached the process and is not repairable from inside a task.
      - The `/healthz`, `/readiness` and `/metrics` note: those three probes take **no** headers by design, and a `200` from `/readiness` beside a `401` from a file call is the expected split.
      - The read, list and delete bullets (`GET /api/v1/files/<path>`, `GET /api/v1/files/?glob=<pattern>`, `DELETE /api/v1/files/<path>`).
      - The "the service owns git, not you" paragraph — the service commits and pushes on every write, `{"ok":true}` means already on the vault's remote, never run `git` against the vault, never resolve a conflict.
      - The 10 MiB body cap.
      - The `/readiness` `503` note — a write that fails is raised to the operator, never retried blindly.
      - The "do not write the daily note" rule for `60 Periodic Notes/Daily/`.

   e. **Change nothing outside the `## Vault` section.** In particular `## Forbidden`'s single named exception (`**One named exception: `vault-obsidian-personal:9090`**` in the network bullet, and `**One named exception: the `X-Gateway-Secret` header on the `## Vault` route**` in the secret-exfiltration bullet) must read exactly as it does today. `## Source repositories` is untouched.

   After this change, `grep -c '^## Vault$' agent/.claude/CLAUDE.md` must still print `1`.

2. **Add `agent/scripts/vault-read.sh`.** Create the `agent/scripts/` directory. The script:

   - is created executable (`chmod 0755 agent/scripts/vault-read.sh`) — the pod's `COPY agent/ /agent/` preserves the mode git records, and § Vault invokes it as a program, so a 0644 file is unreachable in the pod;
   - starts with a `#!/bin/bash` shebang and a short comment header naming its purpose and usage (`vault-read.sh <repo-relative-path>`);
   - sets `set -e -u -o pipefail`;
   - requires the secret loudly and first, before anything else: `: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"`;
   - validates its argument count and prints a usage line to stderr and exits non-zero when the count is wrong (expected: exactly 1);
   - computes `path="${1// /%20}"`;
   - issues `exec curl -sf --max-time 30` with the two gateway headers (`-H "X-Gateway-Initator: agent-claude"` and `-H "X-Gateway-Secret: $GATEWAY_SECRET"`) against `http://vault-obsidian-personal:9090/api/v1/files/${path}`.

   The base URL is a fixed pod constant here (the precedent took it from `GIT_REST_URL`; this agent serves exactly one vault, so hardcode `http://vault-obsidian-personal:9090`). Do not add a URL environment variable.

3. **Add `agent/scripts/vault-write.sh <repo-relative-path> <local-file>`.** Same header shape and the same `chmod 0755` executable mode as requirement 2. It:

   - sets `set -e -u -o pipefail`;
   - requires the secret loudly and first: `: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"`;
   - validates it received exactly 2 arguments, printing the usage line to stderr and exiting non-zero otherwise;
   - computes `path="${1// /%20}"`;
   - issues `exec curl -sf --max-time 30` with `-X POST`, the two gateway headers, `-H 'Content-Type: application/octet-stream'` and **`--data-binary "@${2}"`** against `http://vault-obsidian-personal:9090/api/v1/files/${path}`.

   `--data-binary` is mandatory — `--data` and `-d` strip newlines and would corrupt every vault file. Do not use them anywhere.

4. **Add `agent/scripts/vault-set-section.sh <repo-relative-path> <heading> <content-file>` — the structural fix.** Same header shape and the same `chmod 0755` executable mode as requirement 2. It GETs the file, replaces OR appends exactly one named section, POSTs the result, and preserves every other byte. Compose it so the splice is testable standalone, with no server:

   a. **A `splice_section` shell function** taking `(file, heading, content-file)` and **emitting the result to stdout**. It must:
      - take the heading argument **verbatim** — do not prefix it, and do not accept a bare name in place of a heading. Vault task and goal files use H1 sections (`# Progress`, `# Impact`, `# Tasks`), so a bare `Progress` must NOT be silently turned into `## Progress`: it would not match the file's `# Progress`, and the function would append a duplicate section instead of replacing the one that exists;
      - match heading lines **exactly**: a line whose content, with trailing whitespace stripped, equals the normalized heading. It must NOT match a line that merely contains the heading text (e.g. `### Progress` or `# Progress` or a line of prose mentioning it);
      - count the matches. **Zero matches → append** the heading line and the content-file's bytes at the end of the file, ensuring exactly one newline separates the previous last line from the new heading (a file that does not end in a newline must gain one before the heading is appended, or the last line and the new heading fuse). **Exactly one match → replace** that heading line and every line after it up to, but not including, the next line whose heading level is the same as or higher than the matched heading — for a `# Progress` match, the next line beginning with `# `; for a `## Progress` match, the next line beginning with `# ` or `## ` — so a section carrying `##`/`###` sub-headings is wholly replaced, not truncated at its first sub-heading. **Two or more matches → fail loudly**: write a message naming the heading and the file to stderr and return a non-zero status; do not guess which one was meant;
      - preserve every byte outside the replaced/inserted region, including the file's own trailing-newline state elsewhere in the file.
   b. **A guard so the function is sourceable without executing.** The script must only run its main path when it is executed directly, not when it is sourced — `if [ "${BASH_SOURCE[0]}" = "$0" ]; then main "$@"; fi`. This is what lets the test in requirement 5 source the script and call `splice_section` alone.
   c. **The main path**, for the direct-invocation case: require the secret loudly and first (`: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"`), validate it received exactly 3 arguments (usage to stderr, non-zero exit otherwise), GET the file via `curl -sf --max-time 30` with the two gateway headers into a temp file (use `mktemp`, and clean it up on exit), call `splice_section` on that temp file and the content-file, write the spliced result to a second temp file, and POST that file with `-X POST -H 'Content-Type: application/octet-stream' --data-binary "@<temp>"` to `http://vault-obsidian-personal:9090/api/v1/files/<path>`. The GET and POST halves must be the same requests `vault-read.sh` and `vault-write.sh` make (the same URL, the same headers, `--max-time 30`, `-sf`).

   The script must be idempotent for a section that already has the content-file's content — splicing the same content twice must produce the same file.

5. **Add `agent/scripts/vault-set-section_test.sh`** — a fixture-driven test of `splice_section` with **no server**. It sources `agent/scripts/vault-set-section.sh` and exercises the function against fixtures under a temp directory it creates and cleans up. Each assertion prints a `PASS`/`FAIL` line naming the case, and the script exits non-zero if any assertion fails. It must assert at least:

   - **replace (vault-shaped, H1):** a fixture whose `# Progress` section sits between a `# Tasks` section above and a `# Related` section below — the heading level a real vault task file uses; after splicing, the output's `# Progress` body is the content-file's bytes, the `# Tasks` and `# Related` sections are present unchanged (assert their exact lines are still there), and the old `# Progress` body is absent. Add a second case in the same fixture family: a `# Impact` fixture carrying a `## Sub` line inside it; after splicing, the whole `# Impact` body is replaced, `## Sub` included, because the boundary is the next line of the same-or-higher heading level.
   - **append:** a fixture with no matching heading, and **no trailing newline** on its last line; after splicing, the new heading and content are present, the prior content is intact, and the prior last line is not fused to the new heading.
   - **duplicate:** a fixture containing the same heading twice; `splice_section` returns non-zero, and the message on stderr names the heading.

6. **Add the `## Unreleased` changelog entry.** `CHANGELOG.md` has no `## Unreleased` section today; line 1 is `# Changelog` and the first `## ` heading is `## v0.19.0` at line 9. Insert a new `## Unreleased` section directly above `## v0.19.0`, holding **exactly one** bullet, prefixed `fix:` (this repairs a data-destroying defect). The bullet must state: that a pod's vault write had no read-modify-write protocol, so the POST body was model-generated text and a single write destroyed a vault task file — sections duplicated, one section replaced wholesale, and the file left truncated mid-sentence; that `agent/.claude/CLAUDE.md` § Vault now defines a write as GET → modify a local copy → POST the whole file and states outright that a write replaces the whole file and that anything omitted is destroyed; and that three helpers under `agent/scripts/` (`vault-read.sh`, `vault-write.sh`, `vault-set-section.sh`) make the protocol the easy path, with the section helper touching exactly one named section and failing loudly on an ambiguous heading rather than guessing.

   Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes. The bullet MUST contain the exact token `vault-set-section.sh` (the fold-check anchor: `grep -c 'vault-set-section.sh' CHANGELOG.md` prints `1`). It MUST NOT contain a credential value of any kind. Do not move, delete or reorder the preamble, and do not edit any existing released section.

7. **Change nothing else.** Do NOT edit `Dockerfile`, `main.go`, any other `.go` file, `go.mod`, `go.sum`, `Makefile*`, `.dark-factory.yaml`, `README.md`, or the repo-root `scripts/` directory. Do NOT add a `Makefile.precommit` target. Do NOT add a package, a binary, a symlink or an alias. Do NOT add a `.sh` file anywhere but `agent/scripts/`.

8. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `agent/.claude/CLAUDE.md` still has exactly one `## Vault` heading and the two `## Forbidden` exceptions are byte-for-byte unchanged; (ii) the rewritten § Vault contains the exact sentence `A write REPLACES the whole file.` and the phrase `read-modify-write`; (iii) all three helper scripts plus the test exist under `agent/scripts/`, are executable, and pass `bash -n`; (iv) `--data-binary` appears in both POST scripts and `--data ` (with a space, i.e. not `--data-binary`) appears in neither; (v) `X-Gateway-Initator` (that spelling) appears in all three helpers and in § Vault, and no header or script uses the corrected `Initiator` spelling — the § Vault note that names `Initiator` in order to forbid it is required by requirement 1d and is not a violation; (vi) `GATEWAY_SECRET` is required with `:?` in all three helpers; (vii) the splice test passes all three cases; and (viii) the only files you touched are `agent/.claude/CLAUDE.md`, the four files under `agent/scripts/`, and `CHANGELOG.md`.
</requirements>

<constraints>
- **Do NOT commit — dark-factory handles git.** Do not run any `git` command: this container is started with `.git` masked (the daemon runs with `--set hideGit=true`; `.dark-factory.yaml` itself sets no `hideGit` key, so the default is `false`), so every `git` invocation fails when it is masked — and a failed command in `<verification>` is a false-positive pass, because the daemon's executor does not check verification exit codes.
- **Do NOT run `git` against the vault**, and do not add any instruction telling the agent to. The `git-rest` service owns the vault's git; the pod never touches it.
- **Never print, log, write to a file, or place in argv the value of `GATEWAY_SECRET`.** It goes into one place only: the `X-Gateway-Secret` header of a request to `http://vault-obsidian-personal:9090`. This is `## Forbidden`'s no-secret-exfiltration rule; the header is its one named exception. A script that echoes the secret for debugging is a defect, not a convenience.
- **The header name is `X-Gateway-Initator`** — spelled `Initator`, not `Initiator`. The misspelling is deliberate and the service requires it. Do not "correct" it in the scripts, in § Vault, or anywhere else.
- **`--data-binary`, never `--data` or `-d`.** `--data` strips newlines and corrupts every vault file. There is no case in this change where `--data` is correct.
- **`agent/scripts/` is the placement.** The scripts go there — not repo-root `scripts/` (which holds image-level Python tools copied individually to `/usr/local/bin/`), and not `/usr/local/bin/`. The runtime stage's `COPY agent/ /agent/` is what puts them in the pod, which is why no `Dockerfile` edit is required. Do not add one.
- **The gateway host is reachable from a pod, not from this container.** This container has no route to `vault-obsidian-personal:9090`, so **no live GET or POST may appear in `<verification>`** and none may be attempted. Every check is a local, fixture-driven assertion. The real end-to-end proof — a live pod write through the gateway, then a re-read of the file showing the other sections intact — is **operator-side and deliberately out of scope for this prompt**; do not attempt it, and do not claim it.
- **Preserve the existing guardrail prose you are not rewriting.** `## Forbidden`'s two named exceptions, the `/healthz`-`/readiness`-`/metrics` no-headers note, the "never print the secret" rule, and the two-refusal wording are load-bearing text other changes depend on. Change only what requirement 1 authorises.
- **No new package, binary, symlink, alias, or credential.** `curl` is already in the image (installed by the `alpine` stage's `apk --no-cache add` line). Nothing here needs installing.
- **No Go code.** If you find yourself tempted to write a Go helper, don't — this change is four shell files, one markdown section and one changelog bullet.
- **Existing tests must still pass.** `ROOTDIR=/workspace make precommit` must exit 0 — it runs `ensure`, `format`, `generate`, `python-check`, `python-test`, `test`, `check` and `addlicense` over the Go and Python trees, none of which your change touches, so a failure there is a signal you edited something you should not have.
- **Do NOT edit `CHANGELOG.md` sections other than the new `## Unreleased` section**, and add exactly one bullet to it.
- **Do not mention the counted tokens in script comments.** `<verification>` items 3, 4, 5 and 7 each require an exact count of `1` per file, so a comment that repeats `X-Gateway-Initator: agent-claude`, `X-Gateway-Secret: $GATEWAY_SECRET`, `--data-binary`, or the `:?` requirement pushes a count to `2` and fails that step; and a comment naming bare `--data` or `-d` — even to forbid it — fails item 6, which requires both absent. A comment header naming the script's purpose and usage does not need those literals.
- **The splice test is not wired into `make precommit`, and that is deliberate.** `Makefile.precommit` globs only `scripts/*.py`, so `agent/scripts/vault-set-section_test.sh` is executed by `<verification>` item 2 and by nothing else — it will not run again on its own. Requirement 7 forbids adding a `Makefile.precommit` target, so record the rot instead: end the `## Unreleased` changelog bullet with a clause stating that `agent/scripts/vault-set-section_test.sh` is run only by hand and is not yet wired into `make precommit`, so the gap is durable and a follow-up can close it.
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`; the value is harmless (the `trivy` target's local `.trivyignore` branch wins) and matches every completed prompt in this repository. No `git` command appears below — `.git` is masked here — and nothing below reaches `vault-obsidian-personal:9090`. (Item 30, `make precommit`, is the repository's own validation command and does fetch Go modules and the trivy DB, exactly as it does in every completed prompt here.)

A note on reading the results: the "must be absent" checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `bash -n agent/scripts/vault-read.sh && bash -n agent/scripts/vault-write.sh && bash -n agent/scripts/vault-set-section.sh && bash -n agent/scripts/vault-set-section_test.sh` — exits 0 (all four scripts are syntactically valid bash).
2. `bash agent/scripts/vault-set-section_test.sh` — exits 0 and prints a `PASS` line for each of the `replace`, `append` and `duplicate` cases. This is the structural proof of the fix, run against fixtures with no server.
3. `grep -cF 'X-Gateway-Initator: agent-claude' agent/scripts/vault-read.sh` — prints `1`; `grep -cF 'X-Gateway-Initator: agent-claude' agent/scripts/vault-write.sh` — prints `1`; `grep -cF 'X-Gateway-Initator: agent-claude' agent/scripts/vault-set-section.sh` — prints `1` (every helper sends the initiator header, with the deliberate spelling).
4. `grep -cF 'X-Gateway-Secret: $GATEWAY_SECRET' agent/scripts/vault-read.sh agent/scripts/vault-write.sh agent/scripts/vault-set-section.sh` — prints `1` for each of the three files (each sends the secret from the environment).
5. `grep -c -- '--data-binary' agent/scripts/vault-write.sh` — prints `1`; `grep -c -- '--data-binary' agent/scripts/vault-set-section.sh` — prints `1` (both POST helpers send the file's bytes verbatim).
6. `! grep -qE -- '--data([^-]|$)' agent/scripts/vault-write.sh agent/scripts/vault-set-section.sh` — exits 0 (neither POST helper uses `--data`/`-d`, which would strip newlines and corrupt the file).
7. `grep -cE ': "\$\{GATEWAY_SECRET:\?' agent/scripts/vault-read.sh agent/scripts/vault-write.sh agent/scripts/vault-set-section.sh` — prints `1` for each of the three files (the secret is required loudly, so a misconfigured pod fails with a clear message instead of a confusing `401`).
8. `out=$(GATEWAY_SECRET=SENTINEL-DO-NOT-ECHO bash agent/scripts/vault-set-section.sh 2>&1); printf '%s' "$out" | grep -q 'SENTINEL-DO-NOT-ECHO' && echo LEAK || echo clean` — prints `clean` (with no arguments the helper takes its usage path, and the secret does not appear in its output; the sentinel is a throwaway literal, not a credential).
9. `! grep -qE 'X-Gateway-Initiator|Initiator:' agent/.claude/CLAUDE.md agent/scripts/vault-read.sh agent/scripts/vault-write.sh agent/scripts/vault-set-section.sh` — exits 0 (no header or script uses the corrected spelling; the `## Vault` note that *names* `Initiator` in order to forbid it is required by requirement 1d and is not a violation).
10. `grep -c '^## Vault$' agent/.claude/CLAUDE.md` — prints `1` (the section was rewritten in place, not duplicated or retitled).
11. `grep -c 'A write REPLACES the whole file' agent/.claude/CLAUDE.md` — prints `1` (the replacement semantics are stated outright).
12. `grep -ci 'read-modify-write' agent/.claude/CLAUDE.md` — prints at least `1` (the protocol is named).
13. `awk '/^## /{sec=$0} /read-modify-write/{print "sits under: " sec}' agent/.claude/CLAUDE.md` — prints `sits under: ## Vault` (the read-modify-write wording is inside § Vault, not somewhere else).
14. `awk '/^## Vault/{f=1;next} /^## /{f=0} f' agent/.claude/CLAUDE.md | grep -c 'vault-set-section.sh'` — prints at least `1` (the section names the helper it is telling the agent to use).
15. `awk '/^## Vault/{f=1;next} /^## /{f=0} f' agent/.claude/CLAUDE.md | grep -c 'vault-write.sh'` — prints at least `1` (the whole-file helper is named).
16. `awk '/^## Vault/{f=1;next} /^## /{f=0} f' agent/.claude/CLAUDE.md | grep -c 'vault-read.sh'` — prints at least `1` (the read half of the protocol is named).
17. `grep -cF '**One named exception: `vault-obsidian-personal:9090`**' agent/.claude/CLAUDE.md` — prints `1` (the `## Forbidden` network exception is byte-for-byte unchanged).
18. `grep -cF 'One named exception: the `X-Gateway-Secret` header on the `## Vault` route' agent/.claude/CLAUDE.md` — prints `1` (the `## Forbidden` secret-exfiltration exception is byte-for-byte unchanged).
19. `grep -cF 'no** headers by design' agent/.claude/CLAUDE.md` — prints `1` (the `/healthz`, `/readiness`, `/metrics` no-headers note survives).
20. `grep -cE '^## Source repositories$' agent/.claude/CLAUDE.md` — prints `1` (the section below § Vault is still there and still titled the same).
21. `test -x agent/scripts/vault-read.sh && test -x agent/scripts/vault-write.sh && test -x agent/scripts/vault-set-section.sh && echo executable` — prints `executable` (all three helpers are executable).
22. `! test -f Dockerfile.bak && grep -c '^COPY agent/ /agent/$' Dockerfile` — prints `1` (the Dockerfile is untouched and still ships the whole `agent/` tree, which is why no edit was needed).
23. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
24. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
25. `grep -c 'vault-set-section.sh' CHANGELOG.md` — prints `1` (the changelog bullet).
26. `awk '/^## /{sec=$0} /vault-set-section.sh/{print "sits under: " sec}' CHANGELOG.md` — prints `sits under: ## Unreleased` (the bullet did not fold into a released section).
27. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (the new section carries exactly one bullet).
28. `grep -c '^## v0.19.0' CHANGELOG.md` — prints `1` (the previously-first released heading is still present, directly below the new section).
29. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|X-Gateway-Secret: [^$]' agent/.claude/CLAUDE.md agent/scripts/*.sh CHANGELOG.md` — exits 0 (no credential value, and no literal secret, in any file this change touches — the only `X-Gateway-Secret` occurrence is the `$GATEWAY_SECRET` variable reference).
30. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared `validationCommand` and the exit code the completion report carries.
</verification>

<success_criteria>
- `agent/.claude/CLAUDE.md` § Vault defines a vault write as read-modify-write (GET → modify a local copy → POST the whole file), states outright that a write replaces the whole file and that anything omitted is destroyed, forbids composing a body from memory, and names the three helper scripts with their argument shapes.
- § Vault is still exactly one `## Vault` heading; `## Forbidden`'s two named exceptions, the `/healthz`-`/readiness`-`/metrics` no-headers note, the two-refusal wording and the "never print the secret" rule are all preserved unchanged; `## Source repositories` is untouched.
- `agent/scripts/vault-read.sh`, `agent/scripts/vault-write.sh` and `agent/scripts/vault-set-section.sh` exist, are executable, pass `bash -n`, require `GATEWAY_SECRET` with `:?`, carry `X-Gateway-Initator: agent-claude` (that spelling) and `X-Gateway-Secret: $GATEWAY_SECRET`, and use `--data-binary` (never `--data`/`-d`) for their POSTs.
- `vault-set-section.sh` exposes a sourceable `splice_section` function that emits to stdout, replaces a single exactly-matched heading (up to the next line of the same-or-higher heading level) or appends when absent, preserves every other byte, and fails loudly on two or more matches; its GET and POST halves make the same requests the read and write helpers make.
- `agent/scripts/vault-set-section_test.sh` passes all three fixture cases — replace (other sections intact), append (no trailing newline, no fused lines), duplicate heading (non-zero exit) — with no server.
- `CHANGELOG.md` gains a `## Unreleased` section with exactly one `fix:` bullet containing `vault-set-section.sh`, directly above `## v0.19.0`, and no existing section is edited.
- No credential value and no literal secret appears in any file this change touches.
- Only `agent/.claude/CLAUDE.md`, the four files under `agent/scripts/`, and `CHANGELOG.md` are modified; `Dockerfile`, every `.go` file, `go.mod`, `go.sum`, `Makefile*`, `README.md` and the repo-root `scripts/` are unchanged.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
