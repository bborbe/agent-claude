---
spec: ["004-github-app-token-mint"]
status: draft
created: "2026-10-09T17:03:42Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. BOTH HALVES ARE LOAD-BEARING. The spec's Desired Behavior 5 and Constraint are
   explicit: the new paragraph must name the helper AND state that this does NOT change
   § Vault. Without the second half the new sentence reads as a contradiction of the
   `## Vault` section's ruling ("Never run `git` yourself, never hold or pass a
   credential"). Requirement 3 is that second half, and verification 6–9 assert the Vault
   section survives byte-for-byte in its key clauses.

2. THE HEADLESS HALF. `agent/.claude/CLAUDE.md` is read by BOTH workloads — the image does
   `COPY agent/ /agent/` (Dockerfile:40) and the same file is mounted for
   `claude-headless`, whose Secret carries no `GITHUB_APP_*`. The spec's Constraint says
   the paragraph must therefore not mislead a headless worker into attempting a push it
   cannot make. Requirement 3 (c) is that half: the capability is present only where the
   environment provides the credential, and a pod without it fails loudly rather than
   pushing anonymously — so the correct response is to report the failure, not to retry
   or to look for another way in. This mirrors Failure Modes row 1.

3. WHY THIS IS NOT A CONTRADICTION OF `## Forbidden`. `## Forbidden` bans "No secret
   exfiltration — never print, log, or transmit env vars, API keys, or credentials". The
   helper holds the credential so the WORKER never has to: the token is minted inside the
   helper process and handed to `git` over a pipe. Requirement 3 (d) restates the ban for
   the new path so the two sections read consistently rather than at odds.

4. NO CODE, NO IMAGE, NO TEST. This is a markdown instruction file. It changes no Go file,
   no Dockerfile, no script. The `make precommit` gate still runs and must pass, but it
   proves nothing about this file's content — which is why the checks below are greps
   against the file itself. The file is NOT validated by any linter in this repository
   (there is no markdown lint target), so the grep assertions are the only guard.

5. HIDE-GIT — `.dark-factory.yaml` sets `workflow: direct` and the daemon log records
   `hideGit=true hideGitSource=arg`, so `.git` is masked and every `git` command fails. No
   `git` command appears in `<verification>`, and `ROOTDIR=/workspace` is passed on the
   `make` call.

6. CHANGELOG OWNERSHIP. This prompt does NOT touch `CHANGELOG.md`. The single `feat:`
   bullet for this capability is written by sibling prompt
   `018-spec-004-install-and-wire-credential-helper.md`. dark-factory appends a generic
   "Update CHANGELOG.md" footer to every prompt; for this prompt that footer CONTRADICTS
   the scope below. Follow the body: do not edit `CHANGELOG.md` here.

7. ANCHORS. The spec's Acceptance Criterion 7 greps for two literal things:
   `git-credential-github-app` (≥ 1 occurrence) and "the surrounding paragraph names
   source repositories". Both are stated as hard requirements, plus a pinned section
   heading (`## Source repositories`) so the file's section count is deterministic (6 → 7).

8. README.md IS DELIBERATELY NOT UPDATED. `docs/dod.md` says "README.md is updated if the
   change affects usage, configuration, or setup". The image gaining a credential helper is
   arguably such a change, so this omission is called out rather than left silent. It is
   deliberate: (a) the sibling prompts 015 (`git`) and 016 (`openssl`) made the same call for
   the same file and neither touched README; (b) README.md does not enumerate the image's
   packages or binaries anywhere — it documents the Config CRD's env vars, the cluster
   heartbeat and the local test path (see its `## Env Vars`, `## Creating a New Agent` and
   `## Local Quick Test` sections), so there is no list for the helper to join; (c) the
   worker-facing documentation this increment actually needs lives in
   `agent/.claude/CLAUDE.md`, which is what the worker reads, and that is this prompt. If
   the reviewer wants a README line anyway, the right place is the `## How It Works` section
   — but it should be a separate prompt, since this one is scoped to the guardrail file.
-->

<summary>
- A worker in the pod is told, where it already reads its instructions, that it can push to a source repository over HTTPS without handling a credential itself.
- The instruction states that `git` is already set up in the image, so the worker has nothing to configure and must not try.
- It states that the credential is supplied by the pod's environment and is turned into a short-lived token by a helper `git` calls on its own, so the worker never sees, prints or stores it.
- It states explicitly that this does not change how the vault is handled: the vault is still read and written through its own service, and the worker still does not run `git` against it.
- It warns that a pod whose environment carries no such credential fails loudly instead of pushing anonymously, so the worker reports the failure rather than retrying or improvising.
- It repeats the standing ban on printing, logging or transmitting credentials for this new path, so the instruction cannot be read as licence to handle the token.
- Nothing else in the guardrails changes: the scope, the forbidden list, the output contract, the tools section and the data rules are untouched.
- No code, no image and no configuration change; this is the worker-facing documentation half of the capability.
</summary>

<objective>
Add a `## Source repositories` section to `agent/.claude/CLAUDE.md` that names `git-credential-github-app`, states that `git` is already configured in the image for `github.com` over HTTPS so a worker has nothing to set up, states that the credential comes from the pod's environment and is minted into a short-lived token the worker never handles, states explicitly that this does NOT change `## Vault`, and warns that a pod without the credential fails loudly rather than pushing anonymously — so the capability the image now carries is discoverable by the worker that has it and cannot be mistaken for a change to the vault rule by the worker that does not.
</objective>

<context>
This repository has no root `CLAUDE.md` in a fresh worktree (`/CLAUDE.md` is in `.gitignore`), so do not look for one. Read `docs/dod.md` — the Definition of Done you are graded against, and the repository's declared `validationPrompt`. Read `.dark-factory.yaml` — it is the authority that travels with the repo (`workflow: direct`, `autoGeneratePrompts: true`, `autoRelease: false`).

Read this file in full before editing — it is the file you change, and it is short (51 lines):

- `agent/.claude/CLAUDE.md` — read every line. Its structure today is `# Agent Guardrails` (line 1), then six `## ` sections in order: `## Scope` (line 5), `## Forbidden` (line 11), `## Output` (line 20), `## Tools` (line 26), `## Data` (line 32), `## Vault` (line 38). The file ends at line 51 with the Vault section's last bullet.

Also read, for context on what you are documenting (do NOT edit either):

- `Dockerfile` — the final `FROM alpine` stage installs `scripts/git_credential_github_app.py` as `/usr/local/bin/git-credential-github-app` and runs `git config --system credential.https://github.com.helper git-credential-github-app`. Sibling prompt `018-spec-004-install-and-wire-credential-helper.md` makes that change; this prompt documents it.
- `scripts/git_credential_github_app.py` — the helper. Sibling prompt `017-spec-004-git-credential-github-app-helper.md` creates it. Read its module docstring so the instruction you write matches the contract it implements: it reads `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM`, mints a token, and writes `username=x-access-token` plus `password=<token>` to `git` on stdout.

Lines you must not disturb (quoted from the current file, so you can confirm they survive):

- `## Forbidden`'s third bullet: `- **No secret exfiltration** — never print, log, or transmit env vars, API keys, or credentials`.
- `## Forbidden`'s first bullet names the one permitted internal host (`vault-obsidian-personal:9090`) and permits the public internet "for documentation, research, and the task's own repository remotes" — your new section must be consistent with that, not a relaxation of it.
- `## Vault`'s ruling bullet: `- **The service owns git, not you.** It clones on startup, pulls periodically, and **commits and pushes on every write** … Never run \`git\` yourself, never hold or pass a credential, and never resolve a conflict: the service does that, on the vault's own default branch`.
- `## Vault`'s last bullet: `- Do not write the daily note — \`60 Periodic Notes/Daily/\` is the highest-collision file in the vault layout this agent serves, and no task requires it`.

Why this change: `claude-interactive` was given a GitHub App identity on 2026-10-09 and the credential was inert until the image gained a helper and a build-time git configuration. The capability is useless if the worker does not know it exists — the worker reads `agent/.claude/CLAUDE.md`, and nothing there today says a push is possible. The second half matters just as much: the same file is read by `claude-headless`, which has no such credential, so a sentence that only says "you can push" would be wrong for that workload and would sit next to a Vault section that forbids the worker from running `git` at all. The new section therefore names the boundary: source repositories yes, the vault no.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no `gh` and no general GitHub client; no change to the App's permissions or installation scope; no prod wiring; no vault path — vault reads and writes stay with the `git-rest` service exactly as § Vault specifies, and this increment gives the worker no vault checkout and no vault credential; no per-call credential for the Claude model or the attention store; no change to `claude-headless` behaviour. In this prompt specifically: do NOT edit `Dockerfile`, do NOT edit `scripts/git_credential_github_app.py` or its test, do NOT edit `Makefile.precommit`, do NOT edit `CHANGELOG.md`, do NOT edit any `.go` file, and do NOT add, remove or reword any existing rule — you append one section and change nothing else.

The execution container runs with `.git` masked (`hideGit`), so no `git` command works here. That is why `<verification>` carries no `git` command and passes `ROOTDIR=/workspace` on the `make` call.

Coding guides (in-container paths, read the ones this change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/claude-md-guide.md` — the house conventions for a `CLAUDE.md` guardrail file: section shape, imperative voice, and how a rule should be scoped.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Append exactly one new top-level section, `## Source repositories`, at the end of `agent/.claude/CLAUDE.md`.** It goes after the `## Vault` section's last bullet (`- Do not write the daily note — …`), separated by a blank line, with the heading spelled exactly `## Source repositories` (two hashes, one space, those words, that capitalisation). Do NOT insert it before `## Vault`, do NOT rename or merge any existing section, and do NOT change the file's first line (`# Agent Guardrails`). The file must end up with exactly seven `## ` headings where it has six today.

2. **Name the helper and state that `git` is already configured, so the worker has nothing to do.** The section must:
   (a) name `git-credential-github-app` exactly — this is the spec's Acceptance Criterion 7 anchor, and it must appear at least once;
   (b) state that the image already configures `git` for `github.com` over HTTPS at build time, so a `git clone` / `git push` against a source repository obtains a token automatically and the worker must NOT run `git config` or any other setup step;
   (c) state that a worker must not add the helper, a `git config` line or a credential of its own — the configuration is part of the image.

3. **State the credential's contract and its boundary, in all four parts.**
   (a) **Where the credential comes from.** State that the pod's environment supplies `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM` (a base64-encoded key), and that the helper mints a short-lived installation token from them on each invocation. There is no cache and nothing to renew — each `git` operation mints its own token.
   (b) **Where the credential goes.** State that the helper hands the token to `git` over the credential-helper protocol on a pipe, and that it is never written to a remote URL, `.git/config`, `~/.git-credentials`, `~/.netrc` or a command line.
   (c) **What this does NOT change.** State explicitly that this does NOT change `## Vault`: the vault is still read and written through the `git-rest` service, the worker still does not run `git` against the vault, and the worker still never holds or passes a vault credential. The section must name `Vault` and read as a scoping of the new capability, not as a relaxation of that rule.
   (d) **What a pod without the credential sees.** State that the capability is present only where the environment provides the credential, and that a pod whose environment carries none (the headless workload's Secret has no such key) makes the helper fail loudly rather than letting `git` fall back to an anonymous request — so the correct response is to report the failure, not to retry, to improvise another authentication path, or to conclude the push succeeded.

4. **Keep the credential ban explicit for this path.** The section must state that the worker must never print, log, copy into a file or otherwise transmit the token — the helper holds it so the worker does not have to, and this is the same rule as `## Forbidden`'s `No secret exfiltration`, not an exception to it. Do NOT name a real credential value anywhere, and do NOT include an example that shows a token.

5. **Change nothing else.** Do NOT edit, reword, reorder or delete any existing line, bullet, heading or blank-line structure. Do NOT touch `## Scope`, `## Forbidden`, `## Output`, `## Tools`, `## Data` or `## Vault`. Do NOT add a second section, a table of contents, or a reference to a file outside this repository. Do NOT edit `Dockerfile`, `CHANGELOG.md`, any `.go` file, any `scripts/` file or `Makefile.precommit`.

6. **Match the file's voice and shape.** Write the section as bullets in the same imperative, second-person register as the rest of the file (the existing sections address the reader directly: "Execute ONLY the task…", "Never run `git` yourself…"). Use backticked identifiers for commands, environment variable names and paths, exactly as the surrounding sections do. Keep it as short as the four parts of requirement 3 allow — this is a rule file, not documentation of the helper's internals.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `git-credential-github-app` appears at least once and the phrase `source repositories` appears at least once; (ii) the new heading is exactly `## Source repositories` and the file now has seven `## ` headings; (iii) the `## Vault` section's ruling bullet, its permitted-host bullet and its last bullet are byte-for-byte unchanged; (iv) `# Agent Guardrails` is still line 1 and `## Forbidden` still carries `No secret exfiltration`; (v) no credential value appears anywhere in the file; and (vi) the only file you touched is `agent/.claude/CLAUDE.md`.
</requirements>

<constraints>
- **This file is read by BOTH workloads.** The image does `COPY agent/ /agent/` and the same file is mounted for `claude-headless`, whose Secret carries no `GITHUB_APP_*`. The new paragraph must be written so a headless worker is not misled into attempting a push it cannot make. Requirement 3 (c) and 3 (d) are that safeguard.
- **Do NOT contradict `## Vault`.** The vault is read and written through the `git-rest` service; the worker never runs `git` against it and never holds or passes a vault credential. The new section must say so explicitly, or it reads as a contradiction of the Vault ruling.
- **Do NOT relax `## Forbidden`.** `No secret exfiltration` still holds, `No package installation` still holds, `No system modification` still holds (which is why the `git` configuration is part of the image and a worker must not run `git config`). The new section must not read as an exception to any of them.
- **The credential arrives as environment variables only** — `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PEM`. No second naming scheme, and no real value anywhere in this file.
- **No code, no image, no configuration.** This prompt changes one markdown file. Do NOT edit `Dockerfile`, `CHANGELOG.md` (the changelog entry for this capability is written by sibling prompt `018-spec-004-install-and-wire-credential-helper.md`), any `.go` file, any `scripts/` file, `go.mod`, `go.sum` or `Makefile.precommit`.
- Change ONLY `agent/.claude/CLAUDE.md`. No other file.
- Do NOT commit — dark-factory handles git. Do not run any `git` command: `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT run `docker`, `kubectl`, `make build`, `make buca` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling.
- Existing tests must still pass.
- Errors in any Go code you would write use `github.com/bborbe/errors` — but this prompt adds no Go code, so this rule applies only if you find yourself tempted to write some: don't.
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`; the value is harmless (the `trivy` target's local `.trivyignore` branch wins) and matches every completed prompt in this repository. No `git` command appears below — `.git` is masked here.

A note on reading the results: checks 9 and 10 are "must be absent" and are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared `validationCommand` and the exit code the completion report carries. Note that no linter in this repository parses markdown, so this command proves the repository is still healthy and nothing about this file's content — checks 2–11 are what guard the content.
2. `grep -c 'git-credential-github-app' agent/.claude/CLAUDE.md` — prints at least `1` (the spec's Acceptance Criterion 7 anchor: the helper is named).
3. `grep -c 'source repositories' agent/.claude/CLAUDE.md` — prints at least `1` (the spec's second Criterion 7 anchor: the surrounding paragraph names source repositories).
4. `grep -c '^## Source repositories' agent/.claude/CLAUDE.md` — prints `1` (the new section exists with the exact heading).
5. `grep -c '^## ' agent/.claude/CLAUDE.md` — prints `7` (six pre-existing sections plus exactly one new one).
6. `grep -c '^## Vault$' agent/.claude/CLAUDE.md` — prints `1` (the Vault section still exists and was not renamed).
7. `grep -c 'The service owns git, not you' agent/.claude/CLAUDE.md` — prints `1` (the Vault ruling bullet is byte-for-byte intact).
8. `grep -c 'vault-obsidian-personal:9090' agent/.claude/CLAUDE.md` — prints `1`; `grep -c 'Do not write the daily note' agent/.claude/CLAUDE.md` — prints `1` (the Vault section's permitted host and its last bullet both survive).
9. `grep -c 'GITHUB_APP_PEM' agent/.claude/CLAUDE.md` — prints at least `1` (the new section names the credential variable); `! grep -q 'GITHUB_APP_PEM=' agent/.claude/CLAUDE.md` — exits 0 (the file names the variable but carries no value for it).
10. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' agent/.claude/CLAUDE.md` — exits 0 (no credential value anywhere in the file).
11. `head -1 agent/.claude/CLAUDE.md` — prints `# Agent Guardrails` (the file's title line is untouched); `grep -c 'No secret exfiltration' agent/.claude/CLAUDE.md` — prints `1` (the `## Forbidden` credential ban is intact); `grep -c 'No system modification' agent/.claude/CLAUDE.md` — prints `1` (the `## Forbidden` system-config ban is intact, which is why the new section must not ask a worker to run `git config`).
</verification>

<success_criteria>
- `agent/.claude/CLAUDE.md` gains exactly one new `## Source repositories` section, appended after the `## Vault` section, and the file now has seven `## ` headings where it had six.
- The section names `git-credential-github-app` and the phrase `source repositories`, and states that `git` is already configured in the image for `github.com` over HTTPS so the worker must run no setup step of its own.
- The section states that `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM` come from the pod's environment, that the helper mints a short-lived token per invocation, and that the token reaches `git` over a pipe and is never written to a remote URL, a config file, a credentials file or a command line.
- The section states explicitly that this does NOT change `## Vault` — the vault is still read and written through the `git-rest` service and the worker still does not run `git` against it.
- The section states that a pod whose environment carries no credential fails loudly rather than pushing anonymously, so the worker reports the failure instead of retrying or improvising.
- The section repeats that the token must never be printed, logged, copied into a file or transmitted, consistent with `## Forbidden`'s `No secret exfiltration`.
- `## Scope`, `## Forbidden`, `## Output`, `## Tools`, `## Data` and `## Vault` are unchanged, and `# Agent Guardrails` is still line 1.
- No credential value appears in the file.
- Only `agent/.claude/CLAUDE.md` is modified; `Dockerfile`, `CHANGELOG.md`, every `.go` file, every `scripts/` file and `Makefile.precommit` are untouched.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
