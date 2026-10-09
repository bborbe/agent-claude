---
status: draft
---

# Summary

- A `claude-interactive` pod can hold a GitHub App credential it currently has no way to use.
- The image gains a mint step: sign a JWT with the App PEM, exchange it for a 1-hour installation token, and hand that token to `git`.
- The token never reaches a remote URL, `.git/config`, `.netrc`, or a process table.
- Workers in the pod can then clone a private repo, push a branch and open a PR — the capability the vault's cluster-worker goal is blocked on.
- No new binary, no `gh`; the image already carries `openssl` (v0.15.0) and `git` (v0.14.0).

# Problem

`claude-interactive` was given a GitHub App identity on 2026-10-09 (`bborbe/nuke#420`): `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` in `values-dev.yaml`, and `GITHUB_APP_PEM` in the `claude-agent` Secret. **That credential is inert.** This image has no GitHub App support at all — a grep for `APP_ID|INSTALLATION_ID|PEM_KEY|GITHUB_APP|RS256` across its non-vendor `.go` returns nothing, and the image ships no `gh`. `openssl` landed in v0.15.0 so an RS256 JWT is *signable*, but nothing signs one, and nothing turns a signed JWT into a token `git` will present.

The result is a credential in the pod that no code path reads. That is the anti-pattern this repo's own CHANGELOG names for `claude-headless` — *"that value was inert … a property of the pinned image, not of the Secret"* — and it blocks the vault's cluster-worker goal at SC5, whose "one real vault task completed end-to-end from a cluster worker" needs a repo write.

# Goal

A worker running in a `claude-interactive` pod, holding only the environment the deployment already gives it, can run `git clone` on a private repository, commit, and `git push` a branch — with no credential written to disk, no credential in any command line, and no operator step inside the pod.

# Non-goals

- **Not a general GitHub client.** No `gh`, no PR-creation wrapper, no issue API. The scope is: make `git` authenticate as the App.
- **Not a change to the App's permissions or installation scope.** The dev App stays `bborbe/go-skeleton` only; the prod App stays `all`. Those are deployment facts, not image facts.
- **Not the prod wiring.** The prod Secret and the prod apply are a separate deployment step.
- **Not a per-call credential for the Claude model or the attention store.** Those are separate secrets with separate mechanisms.
- **Not a change to `claude-headless`.** It pins its own Secret and must not receive this credential.

# Acceptance Criteria

- [ ] The image contains an executable that mints an installation token from a PEM — evidence: `docker run --rm --entrypoint sh <tag> -c 'command -v github-app-token'` prints a path and exits 0.
- [ ] The image contains a git credential helper wired so `git` consults it — evidence: `docker run --rm --entrypoint sh <tag> -c 'git config --get-all credential.https://github.com.helper'` prints the helper name and exits 0.
- [ ] Minting against the real dev App returns a usable token, driven only by `GITHUB_APP_ID` / `GITHUB_APP_INSTALLATION_ID` / `GITHUB_APP_PEM` — evidence: with those three set, `<helper> get` on stdin `protocol=https` / `host=github.com` writes a `password=` line whose value is non-empty and whose length is ≥ 40, and writes nothing to stderr; the token is never printed to stdout by the mint path itself.
- [ ] **Negative:** no credential reaches a file or a process argument — evidence: after a successful mint and a `git ls-remote`, `git -C <repo> config --list | grep -c 'token\|password\|github_pat\|ghs_'` returns 0, and `grep -rc 'ghs_' ~/.git-credentials ~/.netrc 2>/dev/null` returns 0 or the file does not exist.
- [ ] **Negative:** with `GITHUB_APP_PEM` unset, the helper fails loudly rather than silently succeeding — evidence: `<helper> get` with `GITHUB_APP_PEM` unset exits non-zero and writes a diagnostic naming the missing variable.
- [ ] `make precommit` exits 0 — evidence: exit code.
- [ ] The mint is documented where the worker reads it — evidence: `grep -c 'github-app-token' agent/.claude/CLAUDE.md` returns ≥ 1.
- [ ] **Post-Deploy (Rung-2):** a live `claude-interactive` pod on dev can push a branch to `bborbe/go-skeleton` — evidence: a prompt asking the pod to clone, commit and push a probe branch returns a commit sha, and `git ls-remote https://github.com/bborbe/go-skeleton <branch>` from outside the pod resolves to that sha.
  - `deploy_check:` `kubectlnukedev -n dev get pod claude-interactive-0 -o jsonpath='{.spec.containers[0].image}'`
  - `deploy_target:` `$(git rev-parse --short HEAD)`

# Verification

## Container-executable (runs inside the YOLO container at prompt time)

- `make precommit` — lint, vet, errcheck, vulncheck, osv-scanner, gosec, trivy clean
- `make test` — unit suite passes
- `grep -n 'github-app-token' Dockerfile` — the mint executable is installed by the image build
- `grep -n 'credential.https://github.com.helper' Dockerfile` — the helper is wired at build time
- `grep -c 'github-app-token' agent/.claude/CLAUDE.md` — the worker-facing instruction exists

## Operator-executable (runs on the host after PR merge, spec verification ladder)

- `make build` then `docker run --rm --entrypoint sh docker.io/bborbe/agent-claude:<tag> -c 'command -v github-app-token'` — the published image carries it
- `docker run --rm -e GITHUB_APP_ID=… -e GITHUB_APP_INSTALLATION_ID=… -e GITHUB_APP_PEM=… --entrypoint sh <tag> -c 'printf "protocol=https\nhost=github.com\n" | git credential-github-app get'` — a real token comes back
- `BRANCH=dev make secrets` then `BRANCH=dev make apply` in `bborbe/nuke` — the pod picks up the new image and the Secret
- `kubectlnukedev -n dev get pod claude-interactive-0` — `1/1 Running`, `restarts=0`

# Desired Behavior

1. **A single mint entry point exists.** One executable in the image produces a fresh GitHub App installation token on demand, reading `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM` from the environment. It does not cache across invocations, because the token's 1-hour life is shorter than a pod's.

2. **`git` uses it without being told.** `git config --global credential.https://github.com.helper` is set at image build time to the helper, so any `git clone` / `git push` against `github.com` over HTTPS obtains a token automatically. A worker does not have to run a setup step, and a worker that forgets to cannot accidentally operate unauthenticated-but-seemingly-fine.

3. **The credential never lands.** The token is handed to `git` over the credential-helper protocol on stdin/stdout. It is never placed in a remote URL, `.git/config`, `~/.git-credentials`, `~/.netrc`, or an argv. This is what `agent/.claude/CLAUDE.md:51` already requires of the worker, and the mechanism must not contradict the instruction the worker is given.

4. **Failure is loud.** A missing or malformed `GITHUB_APP_PEM`, a missing `GITHUB_APP_ID` / `GITHUB_APP_INSTALLATION_ID`, a rejected JWT, or an installation that does not cover the requested repo all produce a non-zero exit and a diagnostic naming which input was wrong. Silence is not an outcome: a helper that returns empty lets `git` fall back to an anonymous request, which succeeds against public repos and fails against private ones — the failure mode that would make a worker believe it had pushed when it had not.

5. **The worker is told the capability exists.** `agent/.claude/CLAUDE.md` names the mint entry point and states that `git` is already configured, so a worker does not invent its own token handling or decline the task as impossible.

6. **Nothing else changes.** The image's existing binaries, entrypoints, environment contract and `claude-headless` behaviour are unchanged.

# Constraints

- **The credential arrives as environment variables only** — `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PEM` — because that is what `bborbe/nuke#420` already wires. The image must not introduce a second naming scheme.
- **`GITHUB_APP_PEM` is base64-encoded PEM** as delivered by the Secret (`teamvaultFile | base64`). The mint step decodes it; it must not assume raw PEM.
- **No new binary dependency.** `openssl` and `git` are already in the image; `python3` is present. The mint must not add a package that is not already installed, and must not add a Go dependency to the agent binary — this is an image-level capability, not an application one.
- **No `gh`.** Installing the GitHub CLI would widen the image's surface and is not needed to make `git` authenticate.
- **The JWT lifetime must be short.** GitHub rejects an `exp` more than 10 minutes ahead; use ≤ 9 minutes so clock skew cannot invalidate it.
- **The existing `agent/.claude/CLAUDE.md` prohibition stands unchanged** — the credential is supplied by the environment and never written into the checkout.
- **The image build has no CI.** `.github/workflows/` runs `make precommit` only, with no `docker build`, so a Dockerfile change is not proven by a green PR — the operator rung is the only check that the image builds.

# Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| `GITHUB_APP_PEM` unset or empty | Helper exits non-zero, names the missing variable on stderr, writes no `password=` line | Operator confirms the `claude-agent` Secret carries `GITHUB_APP_PEM`; `make secrets` re-applies it |
| PEM present but not base64-decodable | Helper exits non-zero naming the decode step | Operator re-checks the TeamVault entry shape (`teamvaultFile`, not `teamvaultPassword`) |
| JWT rejected by GitHub (401) | Helper exits non-zero, distinguishing a rejected JWT from a network failure | Operator confirms `GITHUB_APP_ID` matches the PEM's App — a mismatched pair is the common cause |
| Installation does not cover the repo | `git` receives a valid token and the *server* refuses with 403/404 | Expected and correct — the scope boundary. A worker reports the refusal rather than retrying |
| Network egress to `api.github.com` blocked | Helper exits non-zero with the transport error | Measured 2026-10-09: the pod reaches github.com and `api.github.com` is reachable over HTTPS; no NetworkPolicy exists in the namespace |
| Token expires mid-operation | Next `git` invocation mints a fresh one; nothing is cached to expire | None — this is the design |
| Two workers mint concurrently | Both receive independent valid tokens; GitHub permits this | None |

# Security / Abuse

- **The PEM is a long-lived credential that mints short-lived ones.** It grants `contents: write` and `pull_requests: write` scoped to the App's installation — on dev, `bborbe/go-skeleton` alone. Withholding `workflows` is load-bearing: a minted token physically cannot push `.github/workflows/`.
- **A compromise of the pod yields the same capability the pod already has**, not more: the token is scoped and expires in an hour, and revocation is a single App uninstall. The exposure is bounded by the installation's repo set, which is why the dev App is deliberately `go-skeleton`-only.
- **The helper must not echo the token.** Any diagnostic path that would print it (a `set -x`, an error message embedding the response body) is a defect, not a convenience.
- **Untrusted input:** the helper reads only environment variables it was given; it takes no argument from the model. A worker cannot steer it to a different App or installation.
- **Blast radius of the Secret:** on master, `claude-interactive` is the only `secretName: claude-agent` consumer. `bborbe/nuke#416` adds a second (`easy-agent-goreleaser`, `ALLOWED_TOOLS: Read,Bash(curl:*)`) — once that merges, this Secret hands a write-capable credential to a check agent, and the Secret must be split first.

# Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | The mint executable + its unit test | 1, 4 | 1, 3, 4, 5 | — |
| 2 | Dockerfile install + credential-helper wiring | 2 | 1, 2, 6 | prompt 1 |
| 3 | `agent/.claude/CLAUDE.md` instruction | 5 | 7 | — |

Rationale: prompt 1 is the capability and carries the failure-mode behaviour; prompt 2 makes it reachable from `git` and is the only Dockerfile change; prompt 3 is independent prose. The `Post-Deploy` AC is the operator rung and is not owned by any prompt.

# Do-Nothing Option

The credential stays inert in the pod. `claude-interactive` continues to hold a GitHub App it cannot use, and the cluster-worker goal's SC5 — *"one real vault task completed end-to-end from a cluster worker"* — stays blocked at its repo-write half, as does the sibling task's SC3. The cost of doing nothing is not a missing nicety: it is that a pod holding a write credential reads as configured while being incapable, which is worse than not wiring it at all, because the next session reasons from the wiring rather than from the capability.
