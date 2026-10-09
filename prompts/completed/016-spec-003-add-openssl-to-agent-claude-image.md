---
status: completed
spec: [003-add-openssl-to-agent-claude-image]
summary: 'Added the openssl package to the alpine stage''s apk line in Dockerfile with an RS256 rationale comment, and added a matching feat: bullet under a new ## Unreleased section in CHANGELOG.md.'
execution_id: agent-claude-openssl-exec-016-spec-003-add-openssl-to-agent-claude-image
dark-factory-version: v0.196.0
created: "2026-10-09T12:49:31Z"
queued: "2026-10-09T12:58:14Z"
started: "2026-10-09T12:58:16Z"
completed: "2026-10-09T13:03:54Z"
branch: dark-factory/add-openssl-to-agent-claude-image
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. NO DEVIATION ON THE GIT FRONT. Unlike sibling spec 002, this spec's own
   Verification section already contains no `git` command: every check in its
   container-executable rung is a text assertion against `Dockerfile` and
   `CHANGELOG.md`, plus `make precommit`. That is exactly what this prompt's
   `<verification>` carries, so nothing had to be rewritten. This container runs
   with `.git` masked (a char device — verified), so no `git` command is emitted
   anywhere below. If a future reader wonders why the spec's Constraints section
   still argues about `hideGit`, it is because spec 002's container rung DID use
   `git diff`; this spec deliberately does not.

2. OPERATOR RUNG IS NOT CARRIED. The spec's Acceptance Criteria include three
   `docker run …` checks against a built image (openssl present, the previous
   binaries still resolve, and a real RS256 sign-and-verify). This container has
   no Docker socket, so those are NOT in `<verification>`; they stay on the
   spec's "Operator-executable" rung. `<verification>` carries only what runs
   here. The signing probe in particular is the capability this increment exists
   for and can only be asserted against a built image.

3. EXIT-STATUS SEMANTICS. Several checks are "must be absent" and are written as
   `grep -c …` per the spec's own evidence lines: a count of `0` makes `grep`
   exit 1, which is the EXPECTED result — the printed count is what matters.
   Only check 1 (`make precommit`) has an exit code that is itself the signal.
   The absence checks are scoped to the two files this increment touches
   (`Dockerfile`, `CHANGELOG.md`); the spec prose that names those tokens lives
   in `specs/`, which this increment does not modify.

4. FAILURE-MODE MAPPING. Of the spec's five Failure Modes rows: "openssl is not
   a package name on this base image" → req 1 (one revertible token; the build
   is operator-verified); "adding openssl pulls in an SSH client transitively" →
   req 3 (add nothing but `openssl`; the `command -v ssh` probe is on the
   operator rung); "the trailing backslash is dropped" → the RUN-line-anchored
   check (verification 3), which is why it is anchored to the RUN line rather
   than a bare substring; "the image grows enough to matter" → no action, not a
   defect; "a credential is wired before this lands" → req 3 + the no-credential
   constraints. Every row is addressed.

5. ANCHORS. The spec greps for two literal tokens as its acceptance anchors:
   `RS256` exactly once in `Dockerfile` (the why-comment) and the exact phrase
   `sign a JWT` exactly once in `CHANGELOG.md` (the fold check). Both are stated
   as hard requirements so the executing agent cannot satisfy the spirit and
   miss the anchor.
-->

<summary>
- The shared `agent-claude` image now ships the `openssl` command, so a pod running it can produce an RS256 signature.
- A pod can sign a JWT with an RS256 private key and print the signature, using only tools the image ships, with no credential present.
- This is the prerequisite for a pod to authenticate to GitHub as a GitHub App, whose installation token is minted by signing a JWT; the image previously shipped no tool that could sign one.
- Nothing else about the image changes: the same shell, git, Node, Python, curl, Claude Code CLI, entrypoint and working directory are still there.
- No credential, key, token, SSH client or second signing library is added, so the image holds nothing to leak.
- The image stays shared by both the interactive and the headless workloads, and both keep working unchanged.
- The change is one package name added to a package-install line that already carries seven, plus a comment recording why it is there.
- No Kubernetes manifest, Config CR, Secret, chart or environment variable is touched.
- No Go source file changes, so no new Go test is added; the checks against a built image are run outside this container.
- A new changelog entry records the capability, so the release that follows carries it.
</summary>

<objective>
Add the `openssl` package to the `apk --no-cache add` line of the `alpine` build stage in `Dockerfile` — the stage the final image is built `FROM` — with a comment recording why it is there, and add the matching `## Unreleased` entry to `CHANGELOG.md`, so that a `claude-interactive` pod can sign a JWT with an RS256 private key and print the signature, with no credential present and no other capability of the image changed.
</objective>

<context>
This repository has no root `CLAUDE.md` in a fresh worktree (`/CLAUDE.md` is in `.gitignore`), so do not look for one. Read `docs/dod.md` — the Definition of Done you are graded against, and the repository's declared `validationPrompt`. Read `.dark-factory.yaml` — it is the authority that travels with the repo (`workflow: direct`, `autoGeneratePrompts: true`, `autoRelease: false`).

Read these files before editing:

- `Dockerfile` — read the whole file. It is short (54 lines) and every line below is quoted from it.
- `CHANGELOG.md` — read the preamble and the top of the file. Line 1 is `# Changelog`; the first `## ` heading is `## v0.14.0` at line 9. There is no `## Unreleased` section today.
- `docs/dod.md` — the Definition of Done.

Current `Dockerfile` facts (verified against the working tree; use these as the baseline your change must preserve):

- Three `FROM` lines: `FROM ${DOCKER_REGISTRY}/golang:1.27.1 AS build` (line 2), `FROM ${DOCKER_REGISTRY}/alpine:3.23 AS alpine` (line 11), and the final `FROM alpine` (line 29) — the final image is built from the **named** `alpine` stage, not from the base `alpine` image.
- The `alpine` stage's one package-installing line, currently:
  ```
  RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git \
   && npm install -g --omit=dev --no-optional @anthropic-ai/claude-code@2.1.286 \
   && npm cache clean --force \
   && apk del npm \
   && rm -rf /root/.npm /tmp/*
  ```
  A comment block sits directly above it (lines 12-22) explaining the pinned Claude Code CLI, why `python3` is installed, and why `git` is installed. `openssl` is not present anywhere in the file today.
- Line 54 is `ENTRYPOINT ["/main", "-v=2"]`. Line 47 is `ENV HOME=/home/claude`. Line 35 is `COPY agent/ /agent/`.
- Directive-line counts today, which must not change: `FROM` = 3, `RUN` = 4, `ENV` = 5, `COPY` = 5, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1. The string `apk --no-cache add` occurs exactly once; the string `apk add` occurs zero times.
- The string `ssh` (case-insensitive) occurs zero times in `Dockerfile` and zero times in `CHANGELOG.md` today, and must stay at zero — the spec's no-credential criterion forbids an `ssh` token anywhere this increment touches.
- The string `RS256` occurs zero times in `Dockerfile` today and must occur exactly once after this change (the why-comment).
- No credential marker — `GITHUB_APP`, `x-access-token`, `deploy-key`, or a `BEGIN … PRIVATE KEY` header — occurs in `Dockerfile` or `CHANGELOG.md` today, and none may appear after this change.

Why this change: the `claude-interactive` pod runs this image. A GitHub App installation token is obtained by signing a JWT with the App's private key and exchanging it at `POST /app/installations/<id>/access_tokens`; every step of that exchange except the signature is already possible in the pod, which carries `curl` and `python3`. The signature is not — the image ships no tool that can produce an RS256 signature. `openssl` lands on the `alpine` stage's existing `apk --no-cache add` line because the final image is built `FROM` that stage; a new `RUN apk add` in the final stage would split package installation across two places. Adding the binary alone is what makes signing possible; the credential, the token-minting helper and the git wiring are a separate increment.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no credential of any kind (no GitHub App, installation token, PEM, deploy key, credential helper, `~/.ssh`, or `GITHUB_APP_*` env); no token-minting helper script, no git credential helper, no `git config` line; no push, clone or any other git behaviour change; no change to the `claude-interactive` wiring (no Config CR, Secret, env var, chart, executor or manifest edit — no Kubernetes manifest exists in this repository and none may be added); no second signing library (`python3` cryptography, `PyJWT`, `gnupg` are all forbidden — `openssl` is the whole addition); no change to the agent's guardrails (no file under `agent/` may be edited); and no change to `github.com/bborbe/agent` or to `go.mod`/`go.sum`.

The execution container cannot build an image: it has no Docker socket and no `kubectl`. It also runs with `.git` masked (`hideGit`), so no `git` command works here. That is why every check in `<verification>` is a text assertion against the files this prompt changes, and why the image-level probes (`docker run --rm --entrypoint openssl <image> version`, the resolve check for the previously shipped binaries, and the real RS256 sign-and-verify) are run on the operator side, exactly as the spec's Verification section splits them.

Coding guides (in-container paths, read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — canonical Dockerfile shape, and its "When to use alpine-with-tooling instead" table, which lists the packages an alpine runtime carries when the binary shells out at runtime.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement, the required conventional prefix, the frozen-preamble rule, and the anti-patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Add `openssl` to the `alpine` stage's existing package line — one token, nothing else on that line.** In `Dockerfile`, change the package-install line from:
   ```
   RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git \
   ```
   to:
   ```
   RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git openssl \
   ```
   Append `openssl` as the LAST token before the trailing backslash, after `git`; do not reorder, remove or rename any existing package. Do NOT add a second `RUN apk add` anywhere — in particular not in the final `FROM alpine` stage, which would split package installation across two places. Do NOT change the `npm install`, `npm cache clean`, `apk del npm` or `rm -rf` lines that follow on the same `RUN`.

   If the image build is attempted and `openssl` fails to install or the package name is wrong, the recovery is to revert this one token — the change is deliberately a single token on an existing line. The build itself is not runnable in this container; the build check is on the operator rung of the spec's Verification.

2. **Record why `openssl` is there, in a new paragraph in the comment block above the line.** The comment block at lines 12-22 already explains the pinned Claude Code CLI, `python3`, and `git`. Append a paragraph to that block, in the same style (prose sentences, backticked identifiers, em dashes), stating: `openssl` is installed so a pod can produce an RS256 signature, because a GitHub App installation token is obtained by signing a JWT with the App's private key and the image previously shipped no tool that could sign one; this is the binary only — no credential, no key, no token-minting script — the increment that wires the credential being a separate change.

   The paragraph MUST contain the exact token `RS256` (the spec's grep anchor: `grep -c 'RS256' Dockerfile` prints `1`, so it must occur exactly once in the whole file). It MUST NOT contain the token `ssh` (case-insensitive), and MUST NOT contain `GITHUB_APP`, `x-access-token`, `deploy-key`, or a `BEGIN … PRIVATE KEY` header. Write `GitHub App` with a space, never `GITHUB_APP`. Suggested wording, which you may polish but not reword past these constraints:
   ```
   # `openssl` is installed so a pod can produce an RS256 signature. A GitHub App
   # installation token is obtained by signing a JWT with the App's private key,
   # and the image shipped no tool that could sign one. This is the binary only:
   # no credential, no key, no token-minting script — the increment that wires the
   # credential is a separate change.
   ```
   Leave the existing `git` paragraph (and the rest of the block) exactly as it is; only append.

3. **Do not widen the image.** Do NOT add `openssh-client`, `ssh`, `gnupg`, `tini`, `python3`-cryptography, `PyJWT` or any other package. Do NOT add a credential, a credential helper, a `~/.ssh` directory, a `GITHUB_APP_*` environment variable, or any new `ENV` line. Do NOT add a helper script or a `COPY` of one. Alpine's `openssl` package is expected not to pull in an SSH client; the acceptance criterion asserts that `command -v ssh` prints `absent` in the built image (operator rung). If `ssh` does appear in the built image, the package set is narrowed rather than accepted silently — but do not pre-emptively add or remove anything to avoid it, and do not add anything that is not `openssl`.

4. **Change nothing else in the image.** The `agent-claude` image is shared: `claude-interactive` runs it and so does `claude-headless`. Do NOT touch `ENTRYPOINT`, `CMD`, `WORKDIR`, any `ENV`, any `COPY`, the `FROM` lines, the `LABEL`, or the `agent/` tree.

5. **Add the `## Unreleased` changelog entry.** `CHANGELOG.md` has no `## Unreleased` section today: line 1 is `# Changelog` and the first `## ` heading is `## v0.14.0` at line 9. Insert a new `## Unreleased` section directly above `## v0.14.0`, holding exactly one bullet, prefixed `feat:` (this is a new capability, so it takes the minor bump). State that `openssl` is installed in the `agent-claude` image so a `claude-interactive` pod can sign a JWT with an RS256 private key and print the signature; that minting a GitHub App installation token means signing a JWT with the App's private key and exchanging it for a token, and the image previously shipped no tool that could sign one; and that this is the binary alone, with no credential, no key, no token-minting script and no git change, so a pod can sign before any credential exists, the increment that wires a credential being a separate change. Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes.

   The bullet MUST contain the exact phrase `sign a JWT` (the spec's fold-check anchor: `grep -c 'sign a JWT' CHANGELOG.md` prints `1`). It MUST NOT contain the token `ssh` (case-insensitive), and MUST NOT contain `GITHUB_APP`, `x-access-token`, `deploy-key`, or a `BEGIN … PRIVATE KEY` header. Do not move, delete or reorder the preamble, and do not edit any existing released section.

6. **Change no Go file and add no test.** No `.go` file, no `go.mod`, no `go.sum`, no `mocks/`, no `scripts/` change. Because no Go code changes, no new Go test is added and no coverage target applies — the existing suite run by `make precommit` is the regression guard, and the boundary this change actually crosses (package resolution inside an `apk` build, and the presence and behaviour of the binary in the built image) is reachable only with Docker, which this container does not have. That boundary is verified by the operator-run image probes the spec names: `docker run --rm --entrypoint openssl <image> version`; `docker run --rm --entrypoint sh <image> -c 'git --version && bash --version >/dev/null && node --version && python3 --version && curl --version >/dev/null && echo rc=0'`; and the real signing probe `docker run --rm --entrypoint sh <image> -c 'openssl genrsa -out /tmp/k.pem 2048 2>/dev/null && echo payload > /tmp/p && openssl dgst -sha256 -sign /tmp/k.pem /tmp/p > /tmp/sig && test -s /tmp/sig && echo SIGNED'`. Do not attempt to run them here.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `openssl` is the only package added and it sits on the existing `apk --no-cache add` line, after `git`, with the trailing backslash intact; (ii) the new comment paragraph contains `RS256` exactly once and no `ssh` token appears in either `Dockerfile` or `CHANGELOG.md`; (iii) the changelog bullet contains `sign a JWT` and sits under `## Unreleased`; (iv) the directive-line counts for `FROM`/`RUN`/`ENV`/`COPY`/`WORKDIR`/`CMD`/`ENTRYPOINT`/`LABEL` are unchanged from the baseline in `<context>`; and (v) the only files you touched are `Dockerfile` and `CHANGELOG.md`.
</requirements>

<constraints>
- **`agent-claude` is a shared image.** `claude-interactive` runs it, and so does `claude-headless`. Adding a binary is additive and changes neither workload's behaviour, but the change must not touch `ENTRYPOINT`, `CMD`, `WORKDIR`, env, or the `agent/` tree — those are shared.
- **The final image is built `FROM` the named `alpine` stage**, not from the base `alpine` image. `openssl` goes on that stage's existing `apk --no-cache add` line; a new `RUN apk add` in the final stage would split package installation across two places. Do not add one.
- **No credential of any kind.** No GitHub App, no installation token, no PEM, no deploy key, no SSH client, no credential helper, no `~/.ssh`, no `GITHUB_APP_*` env. The credential increment follows this one.
- **Do NOT add a second signing library.** No `python3` cryptography package, no `PyJWT`, no `gnupg`. `openssl` is the whole addition; a second signing library would duplicate a capability the image would then carry twice.
- **Do NOT change the `claude-interactive` wiring.** No Config CR change, no new Secret, no new env var, no chart change, no executor change, no manifest edit. No Kubernetes manifest exists in this repository and none may be added.
- **Do NOT change the agent's guardrails** and do NOT add rule text. No file under `agent/` may be edited.
- **Do NOT change `github.com/bborbe/agent`** and do not touch `go.mod`/`go.sum`.
- Change ONLY `Dockerfile` and `CHANGELOG.md`. No other file.
- Do NOT commit — dark-factory handles git. Do not run any `git` command: `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT run `docker`, `kubectl`, `make build`, `make buca` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling.
- Existing tests must still pass.
- Errors in any code you would write use `github.com/bborbe/errors` — but this prompt adds no Go code, so this rule applies only if you find yourself tempted to write some: don't.
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, because `Makefile.variables` otherwise resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`; the value is harmless (the `trivy` target's local `.trivyignore` branch wins) and matches every completed prompt in this repository. No `git` command appears below — `.git` is masked here.

A note on reading the results: a `grep -c` that prints `0` exits with status 1, which is the *expected* result for the "must be absent" checks (4, 5, 6, 7) — the printed count is what matters, not grep's exit status. Only check 1 is a command whose exit code is itself the pass/fail signal.

1. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared `validationCommand` and the exit code the completion report carries; it also runs `trivy fs`, which scans the `Dockerfile`.
2. `grep -c 'apk --no-cache add' Dockerfile` — prints `1` (there is still exactly one package-installing line).
3. `grep -cE '^RUN apk --no-cache add .* git openssl \\$' Dockerfile` — prints `1` (the `openssl` token is the last package on the existing `apk --no-cache add` RUN line, after `git`, and the line still ends with its continuation backslash). This is anchored to the RUN line on purpose: a bare-substring match would also be satisfied by a comment that merely mentions `openssl`, or by an edit that dropped the trailing backslash and left the Dockerfile syntactically broken.
4. `grep -c 'apk add' Dockerfile` — prints `0` (no second `apk add` was introduced anywhere, including the final stage).
5. `grep -ci 'ssh' Dockerfile` — prints `0`.
6. `grep -ci 'ssh' CHANGELOG.md` — prints `0`.
7. `grep -ciE 'GITHUB_APP|x-access-token|deploy-key|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' Dockerfile CHANGELOG.md` — prints a `0` count for each of the two files (two lines of the form `CHANGELOG.md:0` and `Dockerfile:0`).
8. `grep -c 'RS256' Dockerfile` — prints `1` (the comment recording why `openssl` is there).
9. `grep -c 'sign a JWT' CHANGELOG.md` — prints `1` (the changelog bullet).
10. `grep -c '^FROM ' Dockerfile` — prints `3`; `grep -c '^RUN ' Dockerfile` — prints `4`; `grep -c '^ENV ' Dockerfile` — prints `5`; `grep -c '^COPY ' Dockerfile` — prints `5`; `grep -c '^WORKDIR ' Dockerfile` — prints `1`; `grep -c '^CMD ' Dockerfile` — prints `1`; `grep -c '^ENTRYPOINT ' Dockerfile` — prints `1`; `grep -c '^LABEL ' Dockerfile` — prints `1`. Every count matches the baseline in `<context>`, i.e. no directive line was added, removed or reordered.
11. `grep -c '^FROM alpine$' Dockerfile` — prints `1` (the final stage still builds `FROM` the named `alpine` stage).
12. `grep -c '^ENTRYPOINT \["/main", "-v=2"\]$' Dockerfile` — prints `1` (the entrypoint is byte-for-byte unchanged).
13. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
14. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
15. `awk '/^## /{sec=$0} /sign a JWT/{print "sits under: " sec}' CHANGELOG.md` — prints `sits under: ## Unreleased` (the bullet did not fold into a released section).
16. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (the new section carries exactly one bullet).
</verification>

<success_criteria>
- `Dockerfile`'s `alpine` stage installs `openssl` alongside the seven packages it already installed, on the same `apk --no-cache add` line, after `git`, with no other line changed.
- A comment above that line records why `openssl` is there — that a GitHub App installation token requires an RS256 signature and the image previously shipped no signing tool — in the style of the existing comment block.
- No `ssh` token and no credential of any kind appears in `Dockerfile` or `CHANGELOG.md`.
- `CHANGELOG.md` gains a `## Unreleased` section with exactly one `feat:` bullet containing the phrase `sign a JWT`, directly above `## v0.14.0`, and no existing section is edited.
- Only `Dockerfile` and `CHANGELOG.md` are modified; no Go file, no manifest and no `agent/` file changes.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
