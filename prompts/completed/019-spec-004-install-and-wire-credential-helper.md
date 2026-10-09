---
status: completed
spec: [004-github-app-token-mint]
summary: 'Installed the git-credential-github-app helper into the agent-claude image (COPY renamed to the exact git-credential-helper name, chmod 0755, build-time git config --system wiring for github.com HTTPS) and added the matching ## Unreleased feat bullet to CHANGELOG.md.'
execution_id: agent-claude-mint-exec-019-spec-004-install-and-wire-credential-helper
dark-factory-version: v0.196.0
created: "2026-10-09T17:03:42Z"
queued: "2026-10-09T17:10:51Z"
started: "2026-10-09T17:20:32Z"
completed: "2026-10-09T17:25:05Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. DEPENDENCY. This prompt is executed AFTER
   `017-spec-004-git-credential-github-app-helper.md`, which creates
   `scripts/git_credential_github_app.py`. Verification 1 asserts that file exists, so a
   ship-order mistake surfaces here rather than as a broken image build months later. The
   COPY in requirement 1 targets the `.py` source path and renames it at COPY time,
   because the AC requires the executable on PATH to be named exactly
   `git-credential-github-app` — there is no second executable and no alias.

2. OPERATOR RUNG IS NOT CARRIED — and this is the important one for THIS prompt. The
   spec's Constraints say `.github/workflows/ci.yml` runs `make precommit` only, with no
   `docker build`, so a Dockerfile change is not proven by a green PR. This container has
   no Docker socket, so an image build cannot happen here either. Every check below is
   therefore a text assertion against `Dockerfile` plus `make precommit`, exactly as the
   spec's own container-executable rung proposes. The three checks that DO require a
   built image — `command -v git-credential-github-app` resolving, `git config --get-all
   credential.https://github.com.helper` printing the helper name, and a real mint — stay
   on the spec's "Operator-executable" rung and are NOT written into `<verification>`.

3. HIDE-GIT — `.dark-factory.yaml` sets `workflow: direct` and the daemon log records
   `hideGit=true hideGitSource=arg`, so `.git` is masked and every `git` command fails.
   No `git` command appears in `<verification>` (which is a little ironic for a prompt
   about git credentials, hence this note), and `ROOTDIR=/workspace` is passed on every
   `make` call.

4. CHANGELOG OWNERSHIP. This prompt owns the ONE `feat:` bullet for this capability.
   Prompt 017 (the helper) and prompt 019 (the worker-facing instruction) deliberately do
   not touch `CHANGELOG.md`, so the feature gets exactly one entry. The generic
   dark-factory "Update CHANGELOG.md" footer will also appear on 017 and 019; those
   prompts tell the agent to follow the body instead.

5. WHY `git config --system` IS IN THE BUILD AND NOT IN THE POD. The spec's Desired
   Behavior 2 and Constraint both force this: `agent/.claude/CLAUDE.md` § Forbidden says
   "No system modification — do not modify /etc, /home, ~/.claude, or system config", so
   a worker cannot be asked to run `git config`. Doing it at build time is the only
   placement that does not require the worker to break its own guardrails. The
   consequence the spec calls out — a worker that forgets to set anything up cannot
   operate unauthenticated-but-seemingly-fine — follows from the helper's loud failure
   paths, which prompt 017 implements.

6. FAILURE-MODE MAPPING for this prompt (the rest are owned by prompt 017): none of the
   spec's Failure Modes rows is a Dockerfile-build failure mode except indirectly — a
   wrong COPY path or a dropped chmod would produce an image where the helper is missing
   or not executable, which the operator rung catches. Requirements 1–3 make that
   impossible by anchoring the COPY source to the exact path prompt 017 creates, and
   verification 1 asserts the source exists.

7. `GITHUB_APP` DOES NOT APPEAR IN `Dockerfile`. The three environment variable NAMES are
   not written into the image anywhere — the deployment supplies them at runtime. The
   Dockerfile only names the helper and the git config key. This keeps the image free of
   the credential contract, which is a deployment fact.

8. `git config --system` writes `/etc/gitconfig`, which holds the helper's NAME only —
   no credential value. The AC's "no credential reaches a file" check greps
   `git config --system --list` for `ghs_|github_pat_|password=` and expects 0, which this
   placement satisfies: the token exists only in the helper process's memory and on the
   stdin/stdout pipe to `git`.
-->

<summary>
- The shared `agent-claude` image now ships the credential helper, installed on `PATH` under exactly the name `git` looks for.
- The image is configured at build time so that any `git` operation against `github.com` over HTTPS obtains an App installation token automatically, with no setup step inside the pod.
- A worker never has to run a configuration command, which matters because the worker's own guardrails forbid modifying system config.
- The token is handed to `git` through the credential-helper protocol, so it never lands in a remote URL, a repository config, a credentials file or a command line.
- The configuration is part of the image, so a worker that forgets to set anything up fails loudly rather than silently operating unauthenticated.
- Nothing else about the image changes: no package is added, no new binary is compiled, the entrypoint and working directory are untouched, and the headless workload is unaffected.
- The published image is the only place this can be proven, so the image-level checks stay with the operator; the automated checks here confirm the image definition is correct.
- A changelog entry records the capability, so the release that follows carries it.
</summary>

<objective>
Install the `git-credential-github-app` helper into the `agent-claude` image at `/usr/local/bin/git-credential-github-app`, make it executable, and point `git` at it for `github.com` over HTTPS with a build-time `git config --system` line, then add the matching `## Unreleased` entry to `CHANGELOG.md` — so that a `claude-interactive` pod holding only the environment the deployment already gives it can run `git clone`, commit and `git push` against a source repository with no credential on disk, none in any command line, and no setup step inside the pod.
</objective>

<context>
This repository has no root `CLAUDE.md` in a fresh worktree (`/CLAUDE.md` is in `.gitignore`), so do not look for one. Read `docs/dod.md` — the Definition of Done you are graded against, and the repository's declared `validationPrompt`. Read `.dark-factory.yaml` — it is the authority that travels with the repo (`workflow: direct`, `autoGeneratePrompts: true`, `autoRelease: false`).

Read these files before editing:

- `Dockerfile` — read the whole file. It is 59 lines and every line below is quoted from it.
- `CHANGELOG.md` — read the preamble and the top of the file. Line 1 is `# Changelog`; the first `## ` heading is `## v0.16.0` at line 9. There is no `## Unreleased` section today.
- `scripts/git_credential_github_app.py` — the helper this prompt installs. It is created by the sibling prompt `017-spec-004-git-credential-github-app-helper.md`; read its module docstring and its subcommand dispatch so you know what you are wiring.
- `docs/dod.md` — the Definition of Done.

Current `Dockerfile` facts (verified against the working tree; use these as the baseline your change must preserve):

- Three `FROM` lines: `FROM ${DOCKER_REGISTRY}/golang:1.27.1 AS build` (line 2), `FROM ${DOCKER_REGISTRY}/alpine:3.23 AS alpine` (line 11), and the final `FROM alpine` (line 34) — the final image is built from the **named** `alpine` stage, not from the base `alpine` image. `git`, `openssl`, `python3` and `curl` are installed by that named stage's single `apk --no-cache add` line (line 28).
- The final stage's existing install-and-configure block is lines 41–53:
  ```
  # The pod-side attention poster and the sibling it loads by path, vendored verbatim
  # from bborbe/claude-supervisor `scripts/`. It is how a cluster worker raises a
  # question or a permission gate to the operator's attention board; `python3` is
  # installed in the `alpine` stage above.
  # ⚠️ The poster resolves its siblings relative to its own directory (`_HERE`), so every
  # file it `_load`s must sit beside it — `answered-attribution.py` today. A missing one
  # fails at import, before argparse runs, so even `--help` dies rather than degrading.
  # ⚠️ Copied, not linked — re-vendor both when upstream changes, or the pod posts with
  # an older protocol.
  COPY scripts/pod-attention.py scripts/answered-attribution.py /usr/local/bin/
  RUN chmod 0755 /usr/local/bin/pod-attention.py
  ENV HOME=/home/claude
  RUN mkdir -p /home/claude/.claude
  ```
  That block is the pattern to follow (a why-comment above a `COPY` into `/usr/local/bin/`, then an explicit `chmod 0755`). Leave it exactly as it is; append your block after it.
- Line 59 is `ENTRYPOINT ["/main", "-v=2"]`. Line 40 is `COPY agent/ /agent/`. Line 52 is `ENV HOME=/home/claude`.
- Directive-line counts today, which this change deliberately alters in exactly two places: `FROM` = 3, `RUN` = 4, `ENV` = 5, `COPY` = 5, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1. The string `apk --no-cache add` occurs exactly once; the string `apk add` occurs zero times.
- The string `git-credential-github-app` occurs zero times in `Dockerfile` and zero times in `CHANGELOG.md` today. The string `credential.https://github.com.helper` occurs zero times in both. The string `GITHUB_APP` occurs zero times in `Dockerfile` today and must stay at zero — the environment variable names are a deployment fact and are not written into the image.
- `scripts/git_credential_github_app.py` is the source path; the executable in the image must be named `git-credential-github-app` (hyphens, no extension).

Why this change: `claude-interactive` was given a GitHub App identity on 2026-10-09 (`GITHUB_APP_ID` and `GITHUB_APP_INSTALLATION_ID` in `values-dev.yaml`, `GITHUB_APP_PEM` in the `claude-agent` Secret) and that credential is inert — no code path reads it. The image already carries `openssl` (so an RS256 JWT is signable, v0.15.0) and `git` (so a clone can happen, v0.14.0). The helper prompt 017 adds is the missing step. This prompt is what makes it reachable: an executable on `PATH`, and a `git config --system` line that means `git` finds it without a worker doing anything. The config goes in the build rather than in the pod because `agent/.claude/CLAUDE.md` § Forbidden forbids a worker from modifying system config, so a worker cannot be asked to run `git config` itself — and a worker that forgets to set anything up cannot therefore operate unauthenticated-but-seemingly-fine.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no `gh` and no general GitHub client; no change to the App's permissions or installation scope (the dev App stays `bborbe/go-skeleton` only, the prod App stays `all` — deployment facts, not image facts); no prod wiring (the prod Secret and apply are a separate deployment step); no vault path (vault reads and writes stay with the `git-rest` service exactly as `agent/.claude/CLAUDE.md` § Vault specifies); no per-call credential for the Claude model or the attention store; no change to `claude-headless` behaviour. In this prompt specifically: do NOT edit `agent/.claude/CLAUDE.md` (sibling prompt 019 does that), do NOT edit `scripts/git_credential_github_app.py` or its test (prompt 017 owns them), do NOT add a package, do NOT add a second `COPY` of `scripts/`, and do NOT touch any `.go` file, `go.mod`, `go.sum` or `Makefile`.

The execution container cannot build an image: it has no Docker socket and no `kubectl`. It also runs with `.git` masked (`hideGit`), so no `git` command works here. That is why every check in `<verification>` is a text assertion against the files this prompt changes, and why the image-level probes (`docker run --rm --entrypoint sh <image> -c 'command -v git-credential-github-app'`, `docker run … git config --get-all credential.https://github.com.helper`, and a real mint) are run on the operator side, exactly as the spec's Verification section splits them.

Coding guides (in-container paths, read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — canonical Dockerfile shape, the `COPY`-then-`chmod` idiom for a script on `PATH`, and the "When to use alpine-with-tooling instead" table.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement, the required conventional prefix, the frozen-preamble rule, and the anti-patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Install the helper, renaming it at COPY time to the exact name `git` looks for.** In `Dockerfile`'s final `FROM alpine` stage, insert a new block immediately after the existing `RUN chmod 0755 /usr/local/bin/pod-attention.py` line and immediately before `ENV HOME=/home/claude`. Add a comment in the style of the surrounding block explaining *why* the helper is there, then:
   ```dockerfile
   COPY scripts/git_credential_github_app.py /usr/local/bin/git-credential-github-app
   ```
   The destination has no trailing slash and is a full file path, so Docker writes the source to exactly `/usr/local/bin/git-credential-github-app`. Do NOT copy `scripts/` wholesale, do NOT add a second `COPY` for the test file, and do NOT leave the executable named `git_credential_github_app.py` — the credential-helper protocol requires the name `git-credential-github-app` and the spec forbids a second executable or an alias.

   The comment must state that the helper mints a GitHub App installation token from the environment and serves the git credential-helper protocol, so `git` obtains a token without the credential ever reaching a remote URL, a config file or a command line. It MUST NOT contain the string `GITHUB_APP` (the variable names are a deployment fact and stay out of the image) and MUST NOT contain a credential value of any kind.

2. **Make it executable with an explicit `chmod`, immediately after the `COPY`.** Add:
   ```dockerfile
   RUN chmod 0755 /usr/local/bin/git-credential-github-app
   ```
   Do not rely on the source file's mode surviving the build, and do not fold this `chmod` into the `git config` line below — the two are separate concerns and the spec's verification anchors on them separately.

3. **Point `git` at the helper at build time, with a comment recording why it cannot be done at runtime.** Immediately after requirement 2's `chmod`, add a comment block and then exactly this directive line:
   ```dockerfile
   RUN git config --system credential.https://github.com.helper git-credential-github-app
   ```
   The comment must state that this is a BUILD-time configuration and why: `agent/.claude/CLAUDE.md` § Forbidden forbids a worker from modifying system config, so a worker cannot be asked to run `git config` itself — and a worker that forgets to set anything up therefore cannot operate unauthenticated-but-seemingly-fine. It must also state that `git` writes no credential here: `/etc/gitconfig` gains the helper's name only, and the token is minted per invocation and handed over on stdin/stdout.

   `git config --system` writes `/etc/gitconfig`; it needs no `HOME` and creates the file if absent. Do NOT use `git config --global` (that would depend on `HOME` and would be a per-user setting rather than an image-wide one), and do NOT add a second `git config` line for any other key. Do NOT add a `git config --system` line that sets a credential *value* — only the `credential.https://github.com.helper` key is set.

4. **Change nothing else in the image.** Do NOT touch the `FROM` lines, the `LABEL`, the `ARG` lines, `COPY agent/ /agent/`, the `ENTRYPOINT`, the `CMD`, the `WORKDIR`, any `ENV`, the `mkdir` line, the zoneinfo `COPY`, or the pod-attention poster's `COPY`/comment block. Do NOT add a package to the `apk --no-cache add` line (no `openssh-client`, no `gh`, no `gnupg`, nothing) — `openssl`, `git` and `python3` are already there and the spec forbids a new package. Do NOT add a `RUN apk add` anywhere. Do NOT compile or install a second binary. Do NOT change the `claude-headless` path: the image stays shared by both workloads, and installing a helper plus a system git config changes neither workload's entrypoint or environment contract.

   After this change the directive-line counts must be exactly: `FROM` = 3, `RUN` = 6, `ENV` = 5, `COPY` = 6, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1. The string `apk --no-cache add` must still occur exactly once and `apk add` exactly zero times.

5. **Add the `## Unreleased` changelog entry.** `CHANGELOG.md` has no `## Unreleased` section today: line 1 is `# Changelog` and the first `## ` heading is `## v0.16.0` at line 9. Insert a new `## Unreleased` section directly above `## v0.16.0`, holding exactly one bullet, prefixed `feat:` (this is a new capability, so it takes the minor bump). State that the `agent-claude` image now installs `git-credential-github-app` — a helper that mints a GitHub App installation token from `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and the base64-encoded `GITHUB_APP_PEM`, serving the git credential-helper protocol — and that the image build points `git` at it with `git config --system credential.https://github.com.helper` for `github.com` over HTTPS, so a `claude-interactive` pod can clone, commit and push a source repository with the token handed over on stdin/stdout and never written to a remote URL, a config file, a credentials file or a command line. State that the wiring is build-time rather than runtime because a worker is forbidden from modifying system config, and that this completes the pair the binary-only predecessors began — `git` in v0.14.0 and `openssl` in v0.15.0 — the prod Secret and apply remaining a separate deployment step.

   Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes. The bullet MUST contain the exact token `git-credential-github-app` (the spec's fold-check anchor: `grep -c 'git-credential-github-app' CHANGELOG.md` prints `1`). It MUST NOT contain a credential value, and MUST NOT contain the string `GITHUB_APP_PEM=` followed by anything that looks like a value. Do not move, delete or reorder the preamble, and do not edit any existing released section.

6. **Change no Go file and add no test here.** No `.go` file, no `go.mod`, no `go.sum`, no `mocks/`, no `Makefile*` change. The helper's unit test belongs to sibling prompt 017 and already runs under `make precommit` via the `python-test` target that prompt adds. The boundary this prompt crosses — the image build resolving the `COPY` source, the `chmod`, and `git config --system` writing `/etc/gitconfig` inside a built image — is reachable only with Docker, which this container does not have. That boundary is verified by the operator-run image probes the spec names: `docker run --rm --entrypoint sh <image> -c 'command -v git-credential-github-app'`; `docker run --rm --entrypoint sh <image> -c 'git config --get-all credential.https://github.com.helper'` printing `git-credential-github-app`; and `docker run --rm -e GITHUB_APP_ID=… -e GITHUB_APP_INSTALLATION_ID=… -e GITHUB_APP_PEM=… --entrypoint sh <tag> -c 'printf "protocol=https\nhost=github.com\n" | git-credential-github-app get'`. Do not attempt to run them here.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement above against the change you actually made. In particular confirm (i) the `COPY` source path is exactly `scripts/git_credential_github_app.py` and the destination is exactly `/usr/local/bin/git-credential-github-app`; (ii) the `chmod 0755` and the `git config --system` line are both present, in that order, after the pod-attention `chmod` and before `ENV HOME=/home/claude`; (iii) `GITHUB_APP` appears zero times in `Dockerfile`; (iv) the directive-line counts match requirement 4 exactly; (v) `CHANGELOG.md` gains one `## Unreleased` section with exactly one `feat:` bullet containing `git-credential-github-app`, directly above `## v0.16.0`; and (vi) the only files you touched are `Dockerfile` and `CHANGELOG.md`.
</requirements>

<constraints>
- **`agent-claude` is a shared image.** `claude-interactive` runs it, and so does `claude-headless`. Adding a helper and a system git config is additive and changes neither workload's behaviour, but the change must not touch `ENTRYPOINT`, `CMD`, `WORKDIR`, any `ENV`, or the `agent/` tree — those are shared. `claude-headless` pins its own Secret and must not receive this credential; it simply will not have `GITHUB_APP_*` in its environment, and the helper's loud failure path covers that.
- **The credential arrives as environment variables only** — `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PEM`. That is what `bborbe/nuke#420` already wires. The image must not introduce a second naming scheme, and none of those names may be written into `Dockerfile` — the deployment supplies them at runtime.
- **The helper is wired at build time via `git config --system`**, never by the worker at runtime — `agent/.claude/CLAUDE.md` forbids a worker modifying system config. Do not add any instruction or script that runs `git config` in a pod.
- **No new package.** `openssl`, `git` and `python3` are already in the image; the build must not add a package. Do not add `gh`, `openssh-client`, `gnupg`, `tini` or anything else to the `apk --no-cache add` line, and do not add a second `RUN apk add`.
- **No second binary and no alias.** The executable is named exactly `git-credential-github-app`. Do not compile a Go binary, do not add a symlink under another name, and do not add a wrapper.
- **The credential must never land.** The token is handed to `git` over the credential-helper protocol on stdin/stdout. Nothing in the image may write it to a remote URL, `.git/config`, `/etc/gitconfig`, `~/.git-credentials` or `~/.netrc`. The `git config --system` line sets the helper's *name* only.
- **The image build has no CI.** `.github/workflows/ci.yml` runs `make precommit` only, with no `docker build`, so a Dockerfile change is not proven by a green PR — the operator rung is the only check that the image builds. That is why the checks here are text assertions and the build probes are explicitly out of scope.
- **Do NOT edit `agent/.claude/CLAUDE.md`.** The worker-facing instruction is sibling prompt `019-spec-004-document-credential-helper.md`. Do NOT edit `scripts/git_credential_github_app.py`, `scripts/git_credential_github_app_test.py` or `Makefile.precommit` — prompt 017 owns those.
- **Do NOT change the App's permissions or installation scope, and do NOT touch the prod wiring.** Those are deployment facts in `bborbe/nuke`, not image facts. No Kubernetes manifest exists in this repository and none may be added.
- Change ONLY `Dockerfile` and `CHANGELOG.md`. No other file.
- Do NOT commit — dark-factory handles git. Do not run any `git` command: `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT run `docker`, `kubectl`, `make build`, `make buca` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling.
- Existing tests must still pass.
- Errors in any Go code you would write use `github.com/bborbe/errors` — but this prompt adds no Go code, so this rule applies only if you find yourself tempted to write some: don't.
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`; the value is harmless (the `trivy` target's local `.trivyignore` branch wins) and matches every completed prompt in this repository. No `git` command appears below — `.git` is masked here.

A note on reading the results: several checks are "must be absent" and are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `test -f scripts/git_credential_github_app.py && echo present` — prints `present`. This is the ship-order guard: the `COPY` source is the file sibling prompt 017 creates.
2. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared `validationCommand` and the exit code the completion report carries; it also runs `trivy fs`, which scans the `Dockerfile` for secrets.
3. `grep -cE '^COPY scripts/git_credential_github_app\.py /usr/local/bin/git-credential-github-app$' Dockerfile` — prints `1` (the helper is copied under its exact executable name).
4. `grep -cE '^RUN chmod 0755 /usr/local/bin/git-credential-github-app$' Dockerfile` — prints `1` (the helper is made executable with an explicit `chmod`).
5. `grep -cE '^RUN git config --system credential\.https://github\.com\.helper git-credential-github-app$' Dockerfile` — prints `1` (the build wires `git` to the helper for `github.com` over HTTPS).
6. `grep -vE '^[[:space:]]*#' Dockerfile | grep -c 'credential.https://github.com.helper'` — prints `1` (exactly one wiring directive; comment lines are excluded so a why-comment that quotes the key cannot change the count).
7. `grep -vE '^[[:space:]]*#' Dockerfile | grep -c 'git config --system'` — prints `1` (no second system-config directive; comment lines excluded).
8. `grep -c 'git-credential-github-app' Dockerfile` — prints at least `3` (the `COPY` destination, the `chmod`, and the `git config` argument).
9. `grep -vE '^[[:space:]]*#' Dockerfile | grep -c 'git_credential_github_app.py'` — prints `1` (the source path appears in exactly one directive; comment lines excluded, and the test file is not copied into the image).
10. `grep -vE '^[[:space:]]*#' Dockerfile | grep -c 'chmod 0755'` — prints `2` (the pre-existing pod-attention chmod plus the new one; comment lines excluded); `grep -c '^chmod 0755' Dockerfile` — prints `0` (neither `chmod` is a stray top-level line).
11. `grep -c 'apk --no-cache add' Dockerfile` — prints `1` (there is still exactly one package-installing line); `grep -c 'apk add' Dockerfile` — prints `0` (no second `apk add` was introduced anywhere).
11a. `grep -cE '^RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git openssl \\$' Dockerfile` — prints `1` (the pre-existing package line is byte-for-byte unchanged, including its trailing continuation backslash, and no package was added to it — anchored to the RUN line on purpose, because a bare-substring match also passes on a syntactically broken edit that dropped the backslash).
12. `! grep -vE '^[[:space:]]*#' Dockerfile | grep -qE 'gh (install|apt|apk)|github-cli|openssh|gnupg|tini'` — exits 0 (no package was added for this capability, and the GitHub CLI is absent; comment lines are excluded so a why-comment that names those packages — as requirement 4's own wording invites — cannot trip the check).
13. `! grep -q 'GITHUB_APP' Dockerfile` — exits 0 (the environment variable names are a deployment fact and are not written into the image).
14. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' Dockerfile CHANGELOG.md` — exits 0 (no credential value in either file).
15. `grep -c '^FROM ' Dockerfile` — prints `3`; `grep -c '^RUN ' Dockerfile` — prints `6`; `grep -c '^ENV ' Dockerfile` — prints `5`; `grep -c '^COPY ' Dockerfile` — prints `6`; `grep -c '^WORKDIR ' Dockerfile` — prints `1`; `grep -c '^CMD ' Dockerfile` — prints `1`; `grep -c '^ENTRYPOINT ' Dockerfile` — prints `1`; `grep -c '^LABEL ' Dockerfile` — prints `1`. Every count matches requirement 4's stated post-change baseline, i.e. exactly two `RUN` lines and one `COPY` line were added and nothing was removed or reordered.
16. `grep -c '^FROM alpine$' Dockerfile` — prints `1` (the final stage still builds `FROM` the named `alpine` stage).
17. `grep -c '^ENTRYPOINT \["/main", "-v=2"\]$' Dockerfile` — prints `1` (the entrypoint is byte-for-byte unchanged).
18. `grep -c '^COPY scripts/pod-attention.py scripts/answered-attribution.py /usr/local/bin/$' Dockerfile` — prints `1` (the pre-existing poster install is untouched).
19. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
20. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
21. `grep -c 'git-credential-github-app' CHANGELOG.md` — prints `1` (the changelog bullet).
22. `awk '/^## /{sec=$0} /git-credential-github-app/{print "sits under: " sec}' CHANGELOG.md` — prints `sits under: ## Unreleased` (the bullet did not fold into a released section).
23. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (the new section carries exactly one bullet).
24. `grep -c '^## v0.16.0' CHANGELOG.md` — prints `1` (the previously-first released heading is still present, directly below the new section).
</verification>

<success_criteria>
- `Dockerfile`'s final `FROM alpine` stage copies `scripts/git_credential_github_app.py` to exactly `/usr/local/bin/git-credential-github-app`, chmods it `0755`, and runs exactly one `git config --system credential.https://github.com.helper git-credential-github-app` line — each with a comment recording why.
- The new block sits after the pod-attention `chmod` and before `ENV HOME=/home/claude`, and the pod-attention block is unchanged.
- No package is added, no second binary is compiled, no alias or symlink is created, and `GITHUB_APP` appears zero times in `Dockerfile`.
- The directive-line counts are exactly `FROM` = 3, `RUN` = 6, `ENV` = 5, `COPY` = 6, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1; `apk --no-cache add` occurs once and `apk add` zero times; the entrypoint and the `FROM alpine` final stage are byte-for-byte unchanged.
- `CHANGELOG.md` gains a `## Unreleased` section with exactly one `feat:` bullet containing `git-credential-github-app`, directly above `## v0.16.0`, and no existing section is edited.
- No credential value appears in `Dockerfile` or `CHANGELOG.md`.
- Only `Dockerfile` and `CHANGELOG.md` are modified; no Go file, no manifest, no `agent/` file, no `scripts/` file and no Makefile changes.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
