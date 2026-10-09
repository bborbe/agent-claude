---
status: verifying
approved: "2026-10-09T12:46:35Z"
generating: "2026-10-09T12:47:33Z"
prompted: "2026-10-09T12:56:16Z"
verifying: "2026-10-09T13:03:54Z"
branch: dark-factory/add-openssl-to-agent-claude-image
---

## Summary

- The `agent-claude` image gains the `openssl` command, so a pod can produce an RS256 signature.
- This is the prerequisite for a pod to authenticate to GitHub as a GitHub App: minting an installation token requires signing a JWT, and the image currently ships no tool that can sign one.
- The change is the binary alone. No credential, no installation token, no git credential helper, no push — those are the next increment and are named under Non-goals.
- `openssl` lands on the `alpine` stage's existing `apk --no-cache add` line, which the final image is already built `FROM`, so nothing else in the image changes shape.
- Measured on the deployed tag `v0.14.0`: `openssl` absent, `cryptography` absent, `PyJWT` absent; `python3` and `curl` present. The pod can fetch and parse, but cannot sign.

## Problem

A `claude-interactive` pod cannot authenticate to GitHub, and the reason is narrower than "the pod has no credential": the image ships no tool that can produce an RS256 signature. A GitHub App installation token is obtained by signing a JWT with the App's private key and exchanging it at `POST /app/installations/<id>/access_tokens`; every step of that exchange except the signature is already possible in the pod, which has `curl` and `python3`. The signature is not.

This is the last missing piece of the repo-access shape recorded on [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]]. That task's SC2 (a worker reads a named repo's file) is met, and its read half needed no credential at all. Its SC3 (a commit lands where a reader outside the pod sees it) does need one, and a write-capable credential was located and proven on 2026-10-09: the dev App `Ben's Sentry Fix Dev` (App ID 4931539, installation 161400107) covers `bborbe/go-skeleton` and returned HTTP 201 on a zero-residue git-blob write probe. So the credential exists, the network reaches `github.com` from the pod, and the only thing standing between the pod and a working installation token is a signing binary.

⚠️ The credential must not be wired before this lands. A PEM in a pod that cannot sign is an inert write-capable credential — the anti-pattern this repo's sibling `nuke` CHANGELOG names for `claude-headless`, where a token was harmless "only because v0.2.6 predates the gate — a property of the pinned image, not of the Secret". This increment exists so the credential lands into an image that can actually use it.

## Goal

After this work, a `claude-interactive` pod can sign a JWT with an RS256 private key and print the signature, using only tools the image ships — with no credential present and no other capability of the image changed.

## Non-goals

- **Any credential.** No GitHub App, no installation token, no PEM, no deploy key, no `GITHUB_APP_*` environment variable, no Secret change. The credential increment is separate and follows this one.
- **A token-minting helper script.** No `github-app-token` script, no git credential helper, no `git config` line. This increment makes signing possible; wiring signing into git is the next increment's job.
- **Pushing, cloning, or any git behaviour change.** This increment adds a binary and nothing else.
- **Changing the `claude-interactive` wiring.** No Config CR change, no new Secret, no new env var, no chart change, no manifest edit. No Kubernetes manifest exists in this repository and none may be added.
- **A full crypto stack.** No `python3` cryptography package, no `PyJWT`, no `gnupg`. `openssl` is the whole addition; a second signing library would duplicate a capability the image would then carry twice.
- **Changing the agent's guardrails.** No file under `agent/` may be edited.

## Acceptance Criteria

- [ ] The `alpine` stage's package line installs `openssl` as its last token, after `git`, with the line's continuation backslash intact. Evidence: `grep -cE '^RUN apk --no-cache add .* git openssl \\$' Dockerfile` prints `1`.
- [ ] No second package-installing line was introduced. Evidence: `grep -c 'apk add' Dockerfile` prints `0`, and `grep -c 'apk --no-cache add' Dockerfile` prints `1`.
- [ ] No credential, key or token appears anywhere this increment touches. Evidence: `grep -ciE 'GITHUB_APP|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|x-access-token|deploy-key' Dockerfile CHANGELOG.md` prints a `0` count for each of the two files.
- [ ] Every other directive line is unchanged in count, so no directive was added, removed or reordered. Evidence: `grep -c '^FROM ' Dockerfile` prints `3`; `^RUN ` prints `4`; `^ENV ` prints `5`; `^COPY ` prints `5`; `^WORKDIR ` prints `1`; `^CMD ` prints `1`; `^ENTRYPOINT ` prints `1`; `^LABEL ` prints `1`.
- [ ] The entrypoint is byte-for-byte unchanged. Evidence: `grep -c '^ENTRYPOINT \["/main", "-v=2"\]$' Dockerfile` prints `1`.
- [ ] The final image is still built `FROM` the named `alpine` stage. Evidence: `grep -c '^FROM alpine$' Dockerfile` prints `1`.
- [ ] `CHANGELOG.md` gains a `## Unreleased` section holding exactly one `feat:` bullet, and no existing released section is edited. Evidence: `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` prints `1`; `awk '/^## /{sec=$0} /sign a JWT/{print "sits under: " sec}' CHANGELOG.md` prints `sits under: ## Unreleased`.
- [ ] `make precommit` exits 0. Evidence: the command's exit code, which is also the repository's declared validation command.
- [ ] **Container:** the built image contains `openssl` and reports its version. Evidence: `docker run --rm --entrypoint openssl <image> version` exits 0 and prints an `OpenSSL …` string. (No `deploy_check` — these are built-image checks, runnable before any cluster is touched, and the guide excludes build-time/image checks from the Post-Deploy marker.)
- [ ] **Container:** everything the image had before still resolves, so adding `openssl` displaced nothing. Evidence: `docker run --rm --entrypoint sh <image> -c 'git --version && bash --version >/dev/null && node --version && python3 --version && curl --version >/dev/null && echo rc=0'` prints `rc=0`.
- [ ] **Container:** the image can actually sign — the capability this increment exists for, asserted as a behaviour rather than as a binary's presence. Evidence: `docker run --rm --entrypoint sh <image> -c 'openssl genrsa -out /tmp/k.pem 2048 2>/dev/null && echo payload > /tmp/p && openssl dgst -sha256 -sign /tmp/k.pem /tmp/p > /tmp/sig && test -s /tmp/sig && echo SIGNED'` prints `SIGNED`.
- [ ] The comment recording why `openssl` is there is present in `Dockerfile`. Evidence: `grep -c 'RS256' Dockerfile` prints `1`.
- [ ] The `CHANGELOG.md` bullet carries no `ssh` token. Evidence: `grep -ci 'ssh' CHANGELOG.md` prints `0`.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `ROOTDIR=/workspace make precommit` — exits 0; the repository's declared validation command.
- `grep -cE '^RUN apk --no-cache add .* git openssl \\$' Dockerfile` — prints `1`.
- `grep -c 'apk add' Dockerfile` — prints `0`.
- `grep -c 'apk --no-cache add' Dockerfile` — prints `1`.
- `grep -ci 'ssh' Dockerfile` — prints `0`.
- `grep -ciE 'GITHUB_APP|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|x-access-token|deploy-key' Dockerfile CHANGELOG.md` — prints `0` for each file.
- `grep -c '^FROM ' Dockerfile` — prints `3`; likewise `^RUN ` `4`, `^ENV ` `5`, `^COPY ` `5`, `^WORKDIR ` `1`, `^CMD ` `1`, `^ENTRYPOINT ` `1`, `^LABEL ` `1`.
- `grep -c '^ENTRYPOINT \["/main", "-v=2"\]$' Dockerfile` — prints `1`.
- `head -1 CHANGELOG.md` — prints `# Changelog`.
- `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `1`.

A `grep -c` that prints `0` exits with status 1, which is the *expected* result for the "must be absent" checks — the printed count is what matters, not grep's exit status. Only `make precommit` has an exit code that is itself the pass/fail signal.

### Operator-executable (runs on the host after PR merge, spec verification ladder)

- `make build` — the image builds with `openssl` on the package line.
- `docker run --rm --entrypoint openssl <image> version` — prints a version string.
- `docker run --rm --entrypoint sh <image> -c 'openssl genrsa -out /tmp/k.pem 2048 && echo p > /tmp/p && openssl dgst -sha256 -sign /tmp/k.pem /tmp/p | wc -c'` — prints a non-zero byte count, which is the signing capability this increment exists to add.
- `docker run --rm --entrypoint sh <image> -c 'command -v ssh || echo absent'` — still prints `absent`, so the new package did not widen the image.

## Desired Behavior

1. The `alpine` build stage's single `apk --no-cache add` line ends with `git openssl`, so both binaries come from the one package-installing line the image already has.
2. A comment above that line records why `openssl` is there — that a GitHub App installation token requires an RS256 signature and the image previously shipped no signing tool.
3. The final image, built `FROM` the named `alpine` stage, contains `openssl` on `PATH`.
4. The image still contains `git`, `bash`, `node`, `python3`, `curl` and `ca-certificates` at the versions it carried before, so no existing capability is displaced.
5. No SSH client is pulled in: `command -v ssh` still prints `absent` in the built image.
6. No credential, key or token is added to the image, the repository or the CHANGELOG.
7. `CHANGELOG.md` carries one `feat:` bullet under `## Unreleased` describing the capability, so the release that follows carries it. The bullet MUST contain the exact phrase `sign a JWT`, which is the anchor the fold check greps for. It MUST NOT contain the token `ssh` (case-insensitive).
8. Nothing else about the image changes: the same entrypoint, working directory, environment, and `agent/` tree.

## Constraints

- **`agent-claude` is a shared image.** `claude-interactive` runs it, and so does `claude-headless`. Adding a binary is additive and changes neither workload's behaviour, but the change must not touch `ENTRYPOINT`, `CMD`, `WORKDIR`, env, or the `agent/` tree — those are shared.
- **The final image is built `FROM` the named `alpine` stage**, not from the base `alpine` image. `openssl` goes on that stage's existing `apk --no-cache add` line; a new `RUN apk add` in the final stage would split package installation across two places. Do not add one.
- **No credential of any kind.** No GitHub App, no installation token, no PEM, no deploy key, no SSH client, no credential helper, no `~/.ssh`, no `GITHUB_APP_*` env. The credential increment follows this one.
- **Do NOT add a second signing library.** No `python3` cryptography package, no `PyJWT`, no `gnupg`. `openssl` is the whole addition.
- **Do NOT change the `claude-interactive` wiring.** No Config CR change, no new Secret, no new env var, no chart change, no executor change, no manifest edit.
- **Do NOT change the agent's guardrails** and do NOT add rule text. No file under `agent/` may be edited.
- **Do NOT change `github.com/bborbe/agent`** and do not touch `go.mod`/`go.sum`.
- Change ONLY `Dockerfile` and `CHANGELOG.md`. No other file.
- Do NOT commit — dark-factory handles git. Do not run any mutating `git` command (`commit`, `tag`, `push`). ⚠️ **Whether read-only `git` works here depends on how the daemon was started, so do not build a check on it.** This repo's `.dark-factory.yaml` sets no `hideGit` key (verified 2026-10-09: the file carries `workflow: direct`, `pr: false`, `autoMerge: false`, `autoRelease: false`, `autoGeneratePrompts: true`, `defaultBranch: master`, `validationPrompt: docs/dod.md`), so `hideGit` defaults to `false` and `git` is available. The operator's daemon-hygiene rule additionally passes `--set hideGit=true`, which masks it. Sibling spec 002 assumed git **was** available and ran `git diff` in its container rung; the two assumptions are not in conflict — they are the two start-up modes. Every check in this spec is therefore a text assertion against the files it changes, which holds under both.
- Do NOT run `docker`, `kubectl`, `make build`, `make buca` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling.
- Existing tests must still pass.
- Errors in any code you would write use `github.com/bborbe/errors` — but this prompt adds no Go code, so this rule applies only if you find yourself tempted to write some: don't.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `openssl` is not a package name on this base image | The build fails at `apk add`, loudly, before any image is published | Revert the one token. Alpine's `openssl` is a real package on `alpine:3.23`; a failure here means the base moved, and the correct fix is the package's new name, not a different base |
| Adding `openssl` pulls in a transitive the image must not carry (an SSH client) | `command -v ssh` prints something other than `absent` in the built image | The package set is narrowed rather than accepted silently — but do not pre-emptively add or remove anything to avoid it, and do not add anything that is not `openssl` |
| The line's trailing backslash is dropped, leaving the Dockerfile syntactically broken | The build fails on a malformed instruction | The check `grep -cE '^RUN apk --no-cache add .* git openssl \\$' Dockerfile` prints `0`, which is why it is anchored to the RUN line rather than a bare substring — a bare match would also be satisfied by a comment that merely mentions `openssl` |
| The image grows enough to matter for pull time or disk | No behaviour change; the image is pulled once per pod | Not a defect. `openssl` and its dependencies are a small fraction of an image that already carries Node and Python |
| A credential is wired into the pod before this lands | An inert write-capable credential sits in a pod that cannot sign with it | Do not wire it. This increment is the prerequisite; the wiring is the next increment, and the ordering is recorded in the Problem section |

## Security / Abuse

The change adds a general-purpose signing tool to a pod that already holds `bash`, `curl` and `python3` behind a bearer-token gate, and whose agent already has `Bash`/`Write`/`Edit`. `openssl` does not widen what the pod can reach — it cannot exfiltrate anything the pod's existing `curl` could not, and it holds no credential of its own. The capability it adds is the ability to sign with a key that is **not** in the image; the key arrives in the following increment, and this increment is deliberately sequenced first so that the key lands into an image that can use it rather than into one that cannot.

The abuse surface to keep closed is that no key material enters the image: `grep` for private-key and credential markers over `Dockerfile` and `CHANGELOG.md` must return `0`, and the build must not gain a `COPY` of any key, `.netrc` or credential helper.

## Suggested Decomposition

**One prompt.** The change is a single token on an existing line, a comment directly above that line, and one changelog bullet. Splitting it would produce prompts that cannot be verified independently: the comment and the token sit on adjacent lines of the same file, and the changelog bullet is the only thing that makes the capability release. This matches sibling spec 002, whose decomposition is also one prompt.

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `openssl` to the `alpine` stage's `apk --no-cache add` line, the comment above it recording why, and the `## Unreleased` changelog bullet | 1–8 | 1–13 | — |

## Do-Nothing Option

Without this, the credential half of [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]] stays blocked: the App exists, its write access to `bborbe/go-skeleton` is proven, and the pod can reach `github.com` — but it cannot sign the JWT that turns those three facts into a token, so no cluster worker can push a commit and that task's SC3 cannot be met. The cost of doing nothing is therefore not "a missing convenience"; it is the task's last open criterion, and with it the parent goal's SC5 repo-write half.
