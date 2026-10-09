---
status: completed
spec: [002-add-git-to-agent-claude-image]
summary: 'Added the git package to the alpine stage''s apk line in Dockerfile with an explanatory comment, and added an ## Unreleased feat bullet to CHANGELOG.md'
execution_id: agent-claude-add-git-exec-015-spec-002-add-git-to-agent-claude-image
dark-factory-version: v0.196.0
created: "2026-10-08T22:32:58Z"
queued: "2026-10-08T22:38:20Z"
started: "2026-10-08T22:38:21Z"
completed: "2026-10-08T22:41:46Z"
branch: dark-factory/add-git-to-agent-claude-image
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. DEVIATION — the spec's single container-executable check is `git diff origin/master -- Dockerfile`.
   That command CANNOT run in the execution container: this repository runs with `hideGit=true`
   (recorded in `.dark-factory.log` as `hideGit=true hideGitSource=arg`), so `.git` is masked and
   every `git` command fails — `git status` returns `fatal: not a git repository`. This repository's
   own completed prompt `prompts/completed/010-bump-agent-interactive-write-timeout.md` documents the
   same masked `.git`. A failed `git` command in `<verification>` would also be a false-positive pass,
   because the daemon's executor does not check verification exit codes. The check is therefore
   realised in `<verification>` as exact-text assertions on `Dockerfile` (the one package-install
   line, the package count, and the unchanged directive-line counts for `FROM`/`ENV`/`COPY`/
   `WORKDIR`/`CMD`/`ENTRYPOINT`), which is strictly stronger than eyeballing a diff. The spec's own
   Verification section already moves the image probes (`docker run …`) to the operator rung; the
   diff check joins them there.

2. OPEN QUESTION — the spec's Acceptance Criterion "No credential value appears anywhere this
   increment touches. Evidence: `git diff origin/master` contains no `GITHUB_APP`, `PEM`,
   `x-access-token`, `deploy-key` or `ssh` string" is stated against the whole diff. The spec file
   itself (`specs/in-progress/002-add-git-to-agent-claude-image.md`) contains `ssh` and `GITHUB_APP`
   in its Non-goals prose, so read literally against the entire branch diff that criterion cannot
   pass. This prompt constrains only the two files it changes — `Dockerfile` and `CHANGELOG.md` —
   and asserts both are free of those tokens. Whether the spec text itself needs rewording is the
   spec author's call, not this prompt's.

3. FAILURE-MODE MAPPING — of the spec's six Failure Modes rows, three are actionable in a
   binary-only prompt and are carried here as requirements: "git pulls in an SSH client transitively"
   (req 3 + verification), "The added package breaks the image build" (req 1: one token on an
   existing line, revertible; the build itself is operator-verified), and "A repo's content shadows
   the agent's own files" (req 4: the prompt adds no clone path and no wrapper script, so no path
   policy is encoded here — the `/agent/repos` rule belongs to the increment that writes the clone
   instructions). The remaining three — clone outside `/agent`, exceeding the 2Gi ephemeral-storage
   limit, and a pod restart losing the writable-layer clone — are runtime behaviours of a clone this
   increment does not perform; they are covered by the spec's Post-Deploy rung and by the credential
   increment, and this prompt deliberately adds no code that could violate or fix them.
-->

<summary>
- The shared `agent-claude` image now ships the `git` command, so a pod running it can clone a repository.
- A pod can clone a public repository into a directory its own file-reading tools can reach, and read the code, with no credential involved.
- Nothing else about the image changes: the same shell, Node, Python, Claude Code CLI, entrypoint and working directory are still there.
- No SSH client and no credential of any kind are added, so the image cannot reach a private repository and has nothing to leak.
- The image stays shared by both the interactive and the headless workloads, and both keep working unchanged.
- The change is one package name added to a package-install line that already installs five, plus a comment recording why it is there.
- No Kubernetes manifest, Config CR, Secret, chart or environment variable is touched.
- No Go source file changes, so no new Go test is added; the image-level checks against a built image are run outside this container.
- A new changelog entry records the capability, so the release that follows carries it.
</summary>

<objective>
Add the `git` package to the `apk --no-cache add` line of the `alpine` build stage in `Dockerfile` — the stage the final image is built `FROM` — with a comment recording why it is there, and add the matching `## Unreleased` entry to `CHANGELOG.md`, so that a `claude-interactive` pod can clone a named public repository into a directory its own tools can read, with no credential involved and no other capability of the image changed.
</objective>

<context>
This repository has no root `CLAUDE.md` in a fresh worktree (`/CLAUDE.md` is in `.gitignore`), so do not look for one. Read `docs/dod.md` — the Definition of Done you are graded against, and the repository's declared `validationPrompt`. Read `.dark-factory.yaml` — it is the authority that travels with the repo (`workflow: direct`, `autoGeneratePrompts: true`, `autoRelease: false`).

Read these files before editing:

- `Dockerfile` — read the whole file. It is short (51 lines) and every line below is quoted from it.
- `CHANGELOG.md` — read the preamble and the top of the file. Line 1 is `# Changelog`; the first `## ` heading is `## v0.12.1` at line 9. There is no `## Unreleased` section today.
- `docs/dod.md` — the Definition of Done.

Current `Dockerfile` facts (verified against the working tree; use these as the baseline your change must preserve):

- Three `FROM` lines: `FROM ${DOCKER_REGISTRY}/golang:1.27.1 AS build` (line 2), `FROM ${DOCKER_REGISTRY}/alpine:3.23 AS alpine` (line 11), and the final `FROM alpine` (line 25) — the final image is built from the **named** `alpine` stage, not from the base `alpine` image.
- The `alpine` stage's one package-installing line, currently:
  ```
  RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 \
   && npm install -g --omit=dev --no-optional @anthropic-ai/claude-code@2.1.286 \
   && npm cache clean --force \
   && apk del npm \
   && rm -rf /root/.npm /tmp/*
  ```
  A comment block sits directly above it (lines 12-18) explaining the pinned Claude Code CLI and why `python3` is installed. `git` is not present anywhere in the file today.
- Line 50 is `ENTRYPOINT ["/main", "-v=2"]`. Line 43 is `ENV HOME=/home/claude`. Line 31 is `COPY agent/ /agent/`.
- Directive-line counts today, which must not change: `FROM` = 3, `RUN` = 4, `ENV` = 5, `COPY` = 5, `WORKDIR` = 1, `CMD` = 1, `ENTRYPOINT` = 1, `LABEL` = 1. The string `apk --no-cache add` occurs exactly once; the string `apk add` occurs zero times.
- The string `ssh` (case-insensitive) occurs zero times in `Dockerfile` and zero times in `CHANGELOG.md` today, and must stay at zero — the spec's no-credential criterion forbids an `ssh` token anywhere this increment touches.

Why this change: the `claude-interactive` pod runs this image, and the image ships no `git` at all, so the pod cannot clone or read any repository — a credential would not help, because there is nothing to authenticate with. `git` lands on the `alpine` stage's existing `apk --no-cache add` line because the final image is built `FROM` that stage; a new `RUN apk add` in the final stage would split package installation across two places. Adding the `git` binary alone is what makes the read half of the capability exist; authentication, push and any clone-path policy are a separate increment.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no credential of any kind (no GitHub App, installation token, PEM, deploy key, credential helper, `~/.ssh`, or `GITHUB_APP_*` env); no push; no change to the `claude-interactive` wiring (no Config CR, Secret, env var, chart, executor or manifest edit); no `ssh` or `openssh-client`; no guardrail-rule text change; no persistence of the checkout; no change to `github.com/bborbe/agent`; and no instruction teaching the agent when to clone.

The execution container cannot build an image: it has no Docker socket and no `kubectl`. It also runs with `.git` masked (`hideGit`), so no `git` command works here. That is why every check in `<verification>` is a text assertion against the files this prompt changes, and why the image-level probes (`docker run --rm --entrypoint git <image> --version`, `docker run … command -v ssh || echo absent`, and the live-pod clone probes) are run on the operator side, exactly as the spec's Verification section splits them.

Coding guides (in-container paths, read the ones your change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — canonical Dockerfile shape, and its "When to use alpine-with-tooling instead" table, which lists `git` among the packages an alpine runtime carries when the binary shells out to git.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` placement, the required conventional prefix, and the anti-patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Add `git` to the `alpine` stage's existing package line — one token, nothing else on that line.** In `Dockerfile`, change line 19 from:
   ```
   RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 \
   ```
   to:
   ```
   RUN apk --no-cache add ca-certificates curl bash nodejs npm python3 git \
   ```
   Append `git` as the last token before the trailing backslash; do not reorder, remove or rename any existing package. Do NOT add a second `RUN apk add` anywhere — in particular not in the final `FROM alpine` stage, which would split package installation across two places. Do NOT change the `npm install`, `npm cache clean`, `apk del npm` or `rm -rf` lines that follow on the same `RUN`.

   If the image build is attempted and `git` fails to install or the package name is wrong, the recovery is to revert this one token — the change is deliberately a single token on an existing line. The build itself is not runnable in this container; the build check is on the operator rung of the spec's Verification.

2. **Record why `git` is there in the comment block above the line.** The comment block at lines 12-18 already explains the pinned Claude Code CLI and `python3`. Append a paragraph to that block, in the same style (prose sentences, backticked identifiers, em dashes), stating: `git` is installed so a `claude-interactive` pod can clone a public repository into a directory under `/agent` and read it with its own `Read`/`Grep`/`Bash` tools; this is the binary only — no credential, no push, no helper script — because the increment that authenticates is a separate change.

   The paragraph MUST contain the exact phrase `clone a public repository`. It MUST NOT contain the token `ssh` (case-insensitive) — see requirement 3. Suggested wording, which you may polish but not reword past these two constraints:
   ```
   # `git` is installed so a `claude-interactive` pod can clone a public repository
   # into a directory under `/agent` and read it with its own `Read`/`Grep`/`Bash`
   # tools. This is the binary only: no credential, no push, no helper script — the
   # increment that authenticates is a separate change.
   ```

3. **Do not widen the image.** Do NOT add `openssh-client`, `ssh`, `gnupg`, `tini` or any other package. Do NOT add a credential, a credential helper, a `~/.ssh` directory, a `GITHUB_APP_*` environment variable, or any new `ENV` line. Do NOT add a helper script or a `COPY` of one. Alpine's `git` package is expected not to pull in an SSH client; the acceptance criterion asserts that `command -v ssh` prints `absent` in the built image (operator rung). If `ssh` does appear in the built image, the package set is narrowed rather than accepted silently — but do not pre-emptively add anything to avoid it, and do not add anything that is not `git`.

4. **Change nothing else in the image, and add no clone policy.** The `agent-claude` image is shared: `claude-interactive` runs it and so does `claude-headless`. Do NOT touch `ENTRYPOINT`, `CMD`, `WORKDIR`, any `ENV`, any `COPY`, the `FROM` lines, the `LABEL`, or the `agent/` tree. Do NOT add a wrapper script, a clone-path constant, a `git config` line, or any instruction about where a clone should land — this increment makes the binary exist and nothing more; the `/agent/repos` path rule belongs to the increment that writes the clone instructions.

5. **Add the `## Unreleased` changelog entry.** `CHANGELOG.md` has no `## Unreleased` section today: line 1 is `# Changelog` and the first `## ` heading is `## v0.12.1` at line 9. Insert a new `## Unreleased` section directly above `## v0.12.1`, holding exactly one bullet, prefixed `feat:` (this is a new capability, so it takes the minor bump). State that `git` is installed in the `agent-claude` image so a `claude-interactive` pod can clone a public repository into a directory under `/agent` and read it with its own tools; that it is the binary alone, with no credential, no push and no helper script, so a pod can read a public repository before any authentication exists; and that the clone lands in the container's writable layer and is deliberately not persisted, the increment that authenticates over HTTPS being a separate change. Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes. The bullet MUST contain the exact phrase `clone a public repository`, and MUST NOT contain the token `ssh` (case-insensitive). Do not move, delete or reorder the preamble, and do not edit any existing released section.

6. **Change no Go file and add no test.** No `.go` file, no `go.mod`, no `go.sum`, no `mocks/`, no `scripts/` change. Because no Go code changes, no new Go test is added and no coverage target applies — the existing suite run by `make precommit` is the regression guard, and the boundary this change actually crosses (package resolution inside an `apk` build, and the presence of the binary in the built image) is reachable only with Docker, which this container does not have. That boundary is verified by the operator-run image probes the spec names: `docker run --rm --entrypoint git <image> --version`, `docker run --rm --entrypoint sh <image> -c 'bash --version >/dev/null && node --version && python3 --version && echo rc=0'`, and `docker run --rm --entrypoint sh <image> -c 'command -v ssh || echo absent'`. Do not attempt to run them here.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement above against the change you actually made. In particular confirm (i) `git` is the only package added and it sits on the existing `apk --no-cache add` line, (ii) no `ssh` token appears in either `Dockerfile` or `CHANGELOG.md`, (iii) the directive-line counts for `FROM`/`RUN`/`ENV`/`COPY`/`WORKDIR`/`CMD`/`ENTRYPOINT`/`LABEL` are unchanged from the baseline in `<context>`, and (iv) the only files you touched are `Dockerfile` and `CHANGELOG.md`.
</requirements>

<constraints>
- **`agent-claude` is a shared image.** `claude-interactive` runs it, and so does `claude-headless`. Adding a binary is additive and changes neither workload's behaviour, but the change must not touch `ENTRYPOINT`, `CMD`, `WORKDIR`, env, or the `agent/` tree — those are shared.
- **The final image is built `FROM` the named `alpine` stage**, not from the base `alpine` image. `git` goes on that stage's existing `apk --no-cache add` line; a new `RUN apk add` in the final stage would work but would split package installation across two places. Do not add one.
- **No credential of any kind.** No GitHub App, no installation token, no PEM, no deploy key, no SSH client, no credential helper, no `~/.ssh`, no `GITHUB_APP_*` env. Authentication is a separate increment with its own spec, gated on a GitHub App that does not exist yet.
- **Do NOT add `ssh` or `openssh-client`.** The chosen credential is HTTPS-only. No `ssh` token may appear in `Dockerfile` or `CHANGELOG.md`.
- **Do NOT push, and add no push machinery.** This increment clones and reads.
- **Do NOT change the `claude-interactive` wiring.** No Config CR change, no new Secret, no new env var, no chart change, no executor change, no manifest edit. No Kubernetes manifest exists in this repository and none may be added.
- **Do NOT change the agent's guardrails** and do NOT add rule text. No file under `agent/` may be edited.
- **Do NOT make the checkout persistent** and do NOT add a volume, a claim or a mount. The clone lands in the container's writable layer by design.
- **Do NOT change `github.com/bborbe/agent`** and do not touch `go.mod`/`go.sum`.
- **Do NOT teach the agent when to clone.** No wrapper script, no clone-path constant, no `git config` line.
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
3. `grep -cE '^RUN apk --no-cache add .* python3 git \\$' Dockerfile` — prints `1` (the `git` token is the last package on the existing `apk --no-cache add` RUN line, after `python3`, and the line still ends with its continuation backslash). This is anchored to the RUN line on purpose: a bare-substring match would also be satisfied by a comment that merely mentions `python3 git`, or by an edit that dropped the trailing backslash and left the Dockerfile syntactically broken.
4. `grep -c 'apk add' Dockerfile` — prints `0` (no second `apk add` was introduced anywhere, including the final stage).
5. `grep -ci 'ssh' Dockerfile` — prints `0`.
6. `grep -ci 'ssh' CHANGELOG.md` — prints `0`.
7. `grep -ciE 'GITHUB_APP|x-access-token|deploy-key|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' Dockerfile CHANGELOG.md` — prints a `0` count for each of the two files (two lines of the form `CHANGELOG.md:0` and `Dockerfile:0`).
8. `grep -c 'clone a public repository' Dockerfile` — prints `1` (the comment recording why `git` is there).
9. `grep -c 'clone a public repository' CHANGELOG.md` — prints `1` (the changelog bullet).
10. `grep -c '^FROM ' Dockerfile` — prints `3`; `grep -c '^RUN ' Dockerfile` — prints `4`; `grep -c '^ENV ' Dockerfile` — prints `5`; `grep -c '^COPY ' Dockerfile` — prints `5`; `grep -c '^WORKDIR ' Dockerfile` — prints `1`; `grep -c '^CMD ' Dockerfile` — prints `1`; `grep -c '^ENTRYPOINT ' Dockerfile` — prints `1`; `grep -c '^LABEL ' Dockerfile` — prints `1`. Every count matches the baseline in `<context>`, i.e. no directive line was added, removed or reordered.
11. `grep -c '^FROM alpine$' Dockerfile` — prints `1` (the final stage still builds `FROM` the named `alpine` stage).
12. `grep -c '^ENTRYPOINT \["/main", "-v=2"\]$' Dockerfile` — prints `1` (the entrypoint is byte-for-byte unchanged).
13. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
14. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
15. `awk '/^## /{sec=$0} /clone a public repository/{print "sits under: " sec}' CHANGELOG.md` — prints `sits under: ## Unreleased` (the bullet did not fold into a released section).
16. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1` (the new section carries exactly one bullet).
</verification>

<success_criteria>
- `Dockerfile`'s `alpine` stage installs `git` alongside the five packages it already installed, on the same `apk --no-cache add` line, with no other line changed.
- A comment above that line records why `git` is there, in the style of the existing comment block.
- No `ssh` token and no credential of any kind appears in `Dockerfile` or `CHANGELOG.md`.
- `CHANGELOG.md` gains a `## Unreleased` section with exactly one `feat:` bullet, directly above `## v0.12.1`, and no existing section is edited.
- Only `Dockerfile` and `CHANGELOG.md` are modified; no Go file, no manifest and no `agent/` file changes.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
