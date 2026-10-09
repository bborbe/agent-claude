---
status: draft
---

## Summary

- A `claude-interactive` pod can hold a GitHub App credential it currently has no way to use.
- The image gains a mint step: sign a JWT with the App PEM, exchange it for a 1-hour installation token, and hand that token to `git`.
- The token never reaches a remote URL, `.git/config`, `~/.git-credentials`, `.netrc`, or a process argument.
- Workers in the pod can then clone a private source repo, push a branch and open a PR — the capability the vault's cluster-worker goal is blocked on.
- No new binary, no `gh`; the image already carries `openssl` (v0.15.0) and `git` (v0.14.0).

## Problem

`claude-interactive` was given a GitHub App identity on 2026-10-09 (`bborbe/nuke#420`): `GITHUB_APP_ID` and `GITHUB_APP_INSTALLATION_ID` in `values-dev.yaml`, and `GITHUB_APP_PEM` in the `claude-agent` Secret. **That credential is inert.** This image has no GitHub App support at all — a grep for `APP_ID|INSTALLATION_ID|PEM_KEY|GITHUB_APP|RS256` across its non-vendor `.go` returns nothing, and the image ships no `gh`. `openssl` landed in v0.15.0 so an RS256 JWT is *signable*, but nothing signs one, and nothing turns a signed JWT into a token `git` will present.

The result is a credential in the pod that no code path reads. That is the anti-pattern this repo's own CHANGELOG names for `claude-headless` — *"that value was inert … a property of the pinned image, not of the Secret"* — and it blocks the vault's cluster-worker goal at SC5, whose "one real vault task completed end-to-end from a cluster worker" needs a repo write.

## Goal

A worker running in a `claude-interactive` pod, holding only the environment the deployment already gives it, can run `git clone` on a private source repository, commit, and `git push` a branch — with no credential written to disk, no credential in any command line, and no operator step inside the pod.

## Non-goals

- **Not a general GitHub client.** No `gh`, no PR-creation wrapper, no issue API. The scope is: make `git` authenticate as the App.
- **Not a change to the App's permissions or installation scope.** The dev App stays `bborbe/go-skeleton` only; the prod App stays `all`. Those are deployment facts, not image facts.
- **Not the prod wiring.** The prod Secret and the prod apply are a separate deployment step.
- **Not the vault path.** Vault reads and writes stay with the `git-rest` service, exactly as `agent/.claude/CLAUDE.md` § Vault specifies. This increment does not give the worker a vault checkout or a vault credential.
- **Not a per-call credential for the Claude model or the attention store.** Those are separate secrets with separate mechanisms.
- **Not a change to `claude-headless` behaviour.** It pins its own Secret and must not receive this credential.

## Acceptance Criteria

- [ ] The image contains an executable named `git-credential-github-app` — evidence: `docker run --rm --entrypoint sh <image> -c 'command -v git-credential-github-app'` prints a path and exits 0.
- [ ] The image configures `git` to use it for `github.com` over HTTPS — evidence: `docker run --rm --entrypoint sh <image> -c 'git config --get-all credential.https://github.com.helper'` prints `git-credential-github-app` and exits 0.
- [ ] Minting against the real dev App returns a token that GitHub accepts — evidence: with `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM` set, the helper's `get` writes a `password=` line, and using that value as a bearer token `GET https://api.github.com/repos/bborbe/go-skeleton` returns HTTP 200. A hardcoded or fabricated string cannot satisfy the 200.
- [ ] **Negative:** no credential reaches a file or a process argument — evidence: after a successful mint and a `git ls-remote` against `github.com`, `git config --system --list | grep -cE 'ghs_|github_pat_|password='` returns 0, `grep -rcE 'ghs_|BEGIN RSA' ~/.git-credentials ~/.netrc 2>/dev/null` returns 0 or the file does not exist, and `ls -la /proc/*/cmdline` shows no token in any argument.
- [ ] **Negative:** with `GITHUB_APP_PEM` unset, the helper fails loudly rather than letting `git` fall back to an anonymous request — evidence: `get` with `GITHUB_APP_PEM` unset exits non-zero, writes a diagnostic naming `GITHUB_APP_PEM` to stderr, and writes no `password=` line to stdout.
- [ ] `make precommit` exits 0 — evidence: exit code.
- [ ] The capability is documented where the worker reads it, scoped to source repos so it cannot be confused with the vault rule — evidence: `grep -c 'git-credential-github-app' agent/.claude/CLAUDE.md` returns ≥ 1 **and** the surrounding paragraph names source repositories.
- [ ] **Post-Deploy (Rung-2):** a live `claude-interactive` pod on dev can push a branch to `bborbe/go-skeleton` — evidence: a prompt asking the pod to clone, commit and push a probe branch returns a commit sha, and `git ls-remote https://github.com/bborbe/go-skeleton <branch>` run from outside the pod resolves to that sha.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.17.0`

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` — lint, vet, errcheck, vulncheck, osv-scanner, gosec, trivy clean
- `make test` — unit suite passes
- `grep -c 'git-credential-github-app' Dockerfile` — the helper is installed by the image build
- `grep -c 'credential.https://github.com.helper' Dockerfile` — the helper is wired at build time
- `grep -c 'git-credential-github-app' agent/.claude/CLAUDE.md` — the worker-facing instruction exists

### Operator-executable (runs on the host after PR merge, spec verification ladder)

- `make build` then `docker run --rm --entrypoint sh docker.io/bborbe/agent-claude:<tag> -c 'command -v git-credential-github-app'` — the published image carries it
- `docker run --rm -e GITHUB_APP_ID=… -e GITHUB_APP_INSTALLATION_ID=… -e GITHUB_APP_PEM=… --entrypoint sh <tag> -c 'printf "protocol=https\nhost=github.com\n" | git-credential-github-app get'` — a real token comes back
- `BRANCH=dev make secrets` then `BRANCH=dev make apply` in `bborbe/nuke` — the pod picks up the new image and the Secret
- `kubectlnukedev -n dev get pod claude-interactive-0` — `1/1 Running`, `restarts=0`

## Desired Behavior

1. **One mint entry point exists, under one name.** The image installs a single executable, `git-credential-github-app`, on `PATH`. It reads `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM` from the environment. It serves the git credential-helper protocol on the subcommands `get`, `store` and `erase`, and additionally accepts `token` to print a bare installation token for a caller that needs one outside `git`. There is no second executable and no differently-named alias. It does not cache across invocations, because the token's 1-hour life is shorter than a pod's.

2. **`git` uses it without being told.** The image build sets `git config --system credential.https://github.com.helper git-credential-github-app`, so any `git clone` / `git push` against `github.com` over HTTPS obtains a token automatically. **This is a build-time configuration, not a runtime one, and that is forced by the image's own guardrails:** `agent/.claude/CLAUDE.md:16` forbids a worker from modifying system config, so a worker cannot be asked to run `git config` itself. A worker that forgets to set anything up cannot therefore operate unauthenticated-but-seemingly-fine.

3. **The credential never lands.** The token is handed to `git` over the credential-helper protocol on stdin/stdout. It is never placed in a remote URL, `.git/config`, `~/.git-credentials`, `~/.netrc`, or an argv. The rule this serves is `agent/.claude/CLAUDE.md:15` — *"No secret exfiltration — never print, log, or transmit env vars, API keys, or credentials"* — and the mechanism must not contradict the instruction the worker is given.

4. **Failure is loud.** A missing or malformed `GITHUB_APP_PEM`, a missing `GITHUB_APP_ID` / `GITHUB_APP_INSTALLATION_ID`, a rejected JWT, or an installation that does not cover the requested repo all produce a non-zero exit and a diagnostic naming which input was wrong. Silence is not an outcome: a helper that returns empty lets `git` fall back to an anonymous request, which succeeds against public repos and fails against private ones — the failure mode that would make a worker believe it had pushed when it had not.

5. **The worker is told the capability exists, and told its boundary.** `agent/.claude/CLAUDE.md` names `git-credential-github-app`, states that `git` is already configured for source repositories, and states explicitly that this does **not** change § Vault — the vault is still read and written through the `git-rest` service and the worker still does not run `git` against it. Without the second half the new sentence reads as a contradiction of line 47.

6. **Nothing else changes.** The image's existing binaries, entrypoints, environment contract and `claude-headless` behaviour are unchanged.

## Constraints

- **The credential arrives as environment variables only** — `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PEM` — because that is what `bborbe/nuke#420` already wires. The image must not introduce a second naming scheme.
- **`GITHUB_APP_PEM` is base64-encoded PEM** as delivered by the Secret (`teamvaultFile | base64`). The mint step decodes it; it must not assume raw PEM.
- **No new package.** `openssl`, `git` and `python3` are already in the image; `agent/.claude/CLAUDE.md:14` forbids package installation at runtime and the image build must not add a package either. The mint must not add a Go dependency to the agent binary — this is an image-level capability, not an application one.
- **No `gh`.** Installing the GitHub CLI would widen the image's surface and is not needed to make `git` authenticate.
- **The helper is wired at build time via `git config --system`**, never by the worker at runtime — `agent/.claude/CLAUDE.md:16` forbids a worker modifying system config.
- **The JWT lifetime must be short.** GitHub rejects an `exp` more than 10 minutes ahead; use ≤ 9 minutes so clock skew cannot invalidate it.
- **The image build has no CI.** `.github/workflows/ci.yml` runs `make precommit` only, with no `docker build`, so a Dockerfile change is not proven by a green PR — the operator rung is the only check that the image builds.
- **`agent/.claude/CLAUDE.md` is read by both workloads.** The image does `COPY agent/ /agent/` (`Dockerfile:40`) and the same file is mounted for `claude-headless`, whose Secret carries no `GITHUB_APP_*`. The new paragraph must therefore be written so that a headless worker reading it is not misled into attempting a push it cannot make; Failure Modes row 1 covers the runtime outcome.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `GITHUB_APP_PEM` unset or empty (e.g. a `claude-headless` pod, which has no such key) | Helper exits non-zero, names `GITHUB_APP_PEM` on stderr, writes no `password=` line | Operator confirms the `claude-agent` Secret carries `GITHUB_APP_PEM`; `BRANCH=dev make secrets` re-applies it |
| PEM present but not base64-decodable | Helper exits non-zero naming the decode step | Operator re-checks the TeamVault entry shape (`teamvaultFile`, not `teamvaultPassword`) |
| JWT rejected by GitHub (401) | Helper exits non-zero, distinguishing a rejected JWT from a network failure | Operator confirms `GITHUB_APP_ID` matches the PEM's App — a mismatched pair is the common cause |
| Installation does not cover the repo | `git` receives a valid token and the *server* refuses with 403/404 | Expected and correct — the scope boundary. A worker reports the refusal rather than retrying |
| `POST /app/installations/<id>/access_tokens` throttled (403 with rate-limit headers) | Helper exits non-zero and names the throttle rather than reporting a generic auth failure | Operator waits for the window; if a worker's git operations are frequent enough to hit it, that is a finding about the workload, not a reason to cache a token past its life |
| Network egress to `api.github.com` blocked | Helper exits non-zero with the transport error | Measured 2026-10-09: the pod reaches `github.com` and `api.github.com` over HTTPS; no NetworkPolicy exists in the dev namespace |
| Token expires mid-operation | Next `git` invocation mints a fresh one; nothing is cached to expire | None — this is the design |
| Two workers mint concurrently | Both receive independent valid tokens; GitHub permits this | None |

## Security / Abuse

- **The PEM is a long-lived credential that mints short-lived ones.** It grants `contents: write` and `pull_requests: write` scoped to the App's installation — on dev, `bborbe/go-skeleton` alone. Withholding `workflows` is load-bearing: a minted token physically cannot push `.github/workflows/`.
- **A compromise of the pod yields the same capability the pod already has**, not more: the token is scoped and expires in an hour, and revocation is a single App uninstall. The exposure is bounded by the installation's repo set, which is why the dev App is deliberately `go-skeleton`-only.
- **The helper must not echo the token.** Any diagnostic path that would print it — a `set -x`, an error message embedding the response body — is a defect, not a convenience.
- **Untrusted input:** the helper reads only environment variables it was given; it takes no argument from the model that selects an App or installation. A worker cannot steer it to a different identity.
- **Blast radius of the Secret:** on master, `claude-interactive` is the only `secretName: claude-agent` consumer. `bborbe/nuke#416` adds a second (`easy-agent-goreleaser`, `ALLOWED_TOOLS: Read,Bash(curl:*)`) — once that merges, this Secret hands a write-capable credential to a check agent, and the Secret must be split first.

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | The `git-credential-github-app` executable + its unit test | 1, 3, 4 | 1, 3, 4, 5 | — |
| 2 | Dockerfile install + `git config --system` wiring | 2 | 1, 2 | prompt 1 |
| 3 | `agent/.claude/CLAUDE.md` instruction | 5 | 7 | — |

DB6 ("nothing else changes") is an invariant held by all three prompts rather than a behaviour any one of them owns — it is asserted by the full `make precommit` / `make test` run on the combined branch, and by the operator rung's `docker run` checks that the pre-existing binaries still resolve.

The `**Post-Deploy (Rung-2):**` AC is the operator rung and is not owned by any prompt.

**No new scenario.** The runtime-only behaviour — a real installation token authenticating a real `git push` against a real repo — is observed by the Post-Deploy AC against the live dev pod, which is the only layer that can reach it. Unit and integration tests cannot mint against GitHub, and this repo has no `scenarios/` directory. Same rationale as sibling 001.

## Do-Nothing Option

The credential stays inert in the pod. `claude-interactive` continues to hold a GitHub App it cannot use, and the cluster-worker goal's SC5 — *"one real vault task completed end-to-end from a cluster worker"* — stays blocked at its repo-write half, as does the sibling task's SC3. The cost of doing nothing is not a missing nicety: it is that a pod holding a write credential reads as configured while being incapable, which is worse than not wiring it at all, because the next session reasons from the wiring rather than from the capability.
