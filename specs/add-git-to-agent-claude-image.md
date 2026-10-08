## Summary

- The `agent-claude` image gains the `git` binary, so a `claude-interactive` pod can clone a repository into its own tool sandbox and read the code.
- The change is the binary only. No credential, no authentication, no push, no helper script — those are the next increment and are named under Non-goals.
- On its own this makes the pod able to clone and read **public** repositories, which is what [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]] needs for the first half of its SC2 against `bborbe/vault-ui`.
- `git` lands on the existing `apk --no-cache add` line of the `alpine` build stage, which the final image is already built `FROM`, so nothing else in the image changes shape.
- `ssh` is deliberately not added: the chosen credential is a GitHub App installation token over HTTPS, which needs no SSH client.

## Problem

A `claude-interactive` pod cannot read any repository, and the reason is narrower than "the pod has no repo": the image ships no `git` at all.

Measured 2026-10-08: the live pod runs `docker.prod.nuke.benjamin-borbe.de:443/bborbe/agent-claude:v0.12.1` (`kubectlnukedev -n dev get pod claude-interactive-0 -o jsonpath='{.spec.containers[*].image}'`). At that tag the `alpine` stage — the stage the final image is built `FROM` — installs exactly `ca-certificates curl bash nodejs npm python3` and then `apk del npm`; there is no `git` and no `openssh-client`. The one package-installing `RUN` in the `build` stage is discarded, because only `/main` is copied across.

The consequence is already recorded on [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]]: a replayed `/vault-cli:work-on-task` prompt returned `Vault-UI codebase not found in the agent directory`, because there is no checkout and no tool with which to make one. That task's SC2 ("a cluster worker can read a named repo's file from inside the pod") cannot be attempted by any mechanism while `git` is absent — a credential would not help, because there is nothing to authenticate with.

This is the prerequisite increment, and it is the only part of that task's credential work which is **not** blocked on a GitHub App existing.

## Goal

The `claude-interactive` pod can clone a named public repository into a directory its own `Read`/`Grep`/`Bash` tools can reach, and print a named file's real content from that clone — with no credential involved and no other capability of the image changed.

## Non-goals

- **Any credential.** No GitHub App, no installation token, no PEM, no deploy key, no SSH client, no credential helper, no `~/.ssh`, no `GITHUB_APP_*` env. Authentication is a separate increment with its own spec, gated on a GitHub App that does not exist yet.
- **Push.** This increment clones and reads. Writing to a remote is the credential increment's job.
- **Changing the `claude-interactive` wiring.** No Config CR change, no new Secret, no new env var, no chart change, no executor change, no manifest edit.
- **Adding `ssh` or `openssh-client`.** The chosen credential is HTTPS-only.
- **Changing the agent's guardrails** (`agent/.claude/CLAUDE.md`). Cloning from `github.com` is public internet, already permitted by the existing rule; no rule text changes here.
- **Making the checkout persistent.** The clone lands in the container's writable layer by design (see Constraints). Persistence is not attempted.
- **Changing `github.com/bborbe/agent`** — no library surface is added.
- **Teaching the agent that it can clone.** Telling the pod *when* to clone, and what to do with the checkout, is the credential increment's job, where the push policy is also decided. This increment only makes the capability exist.

## Acceptance Criteria

- [ ] **Container:** the built image contains `git` and reports its version. Evidence: `docker run --rm --entrypoint git <image> --version` exits 0 and prints a `git version …` string. (No `deploy_check` — checkable against the built image before any cluster is touched.)
- [ ] **Container:** everything the image had before still resolves. Evidence: `docker run --rm --entrypoint sh <image> -c 'bash --version >/dev/null && node --version && python3 --version && /main --help >/dev/null 2>&1; echo rc=$?'` — the three version commands exit 0, and the image's `ENTRYPOINT` is still `/main`.
- [ ] **Container:** `ssh` is absent, proving the change did not quietly pull in an SSH client. Evidence: `docker run --rm --entrypoint sh <image> -c 'command -v ssh || echo absent'` prints `absent`.
- [ ] **Container:** the diff is the package name and nothing else. Evidence: `git diff origin/master -- Dockerfile` shows one added token (`git`) on the existing `apk --no-cache add` line, with no change to `ENTRYPOINT`, `CMD`, `WORKDIR`, `ENV`, or any `COPY`.
- [ ] **Post-Deploy (Rung-2):** inside the live `claude-interactive` pod, a prompt asking the worker to run `git --version` returns a real version string rather than a command-not-found. Evidence: the prompt text and the returned version string quoted in the results.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] **Post-Deploy (Rung-2):** the pod clones the public repo `bborbe/vault-ui` into a path inside its tool sandbox and prints a named file's real content, with **no credential** used or present. Evidence: the prompt, the clone path (which must be under `/agent`), the named file, and its content quoted verbatim.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] **Post-Deploy (Rung-2):** the clone is readable by the agent's own tools, not merely present on disk — the failure mode where a clone outside the sandbox succeeds and is then unreadable. Evidence: in the same turn, `Read` (or `Grep`) on a file inside the clone returns content, and the path used is under `/agent`.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] **Post-Deploy (Rung-2):** the clone is writable — this increment must NOT copy `repo-clone.sh`'s read-only hardening (`chmod a-w` over the tree including `.git`), because the credential increment must push from this same checkout. Evidence: after the clone, a file created inside the worktree can be removed again.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] **Post-Deploy (Rung-2):** the pod is still `1/1 Running` with `0 restarts` after the probes, and a second sequential prompt is answered, proving the session is still held across requests. Evidence: `kubectlnukedev -n dev get pod claude-interactive-0` reads `1/1 Running` and `0` restarts, and the second prompt's reply references the first.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] No credential value appears anywhere this increment touches. Evidence: `git diff origin/master` contains no `GITHUB_APP`, `PEM`, `x-access-token`, `deploy-key` or `ssh` string, and `kubectlnukedev -n dev logs claude-interactive-0 | grep -ci 'x-access-token\|Authorization: Bearer'` prints `0`.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `git --version` exits 0 and prints a version.
- `bash --version`, `node --version`, `python3 --version` each still exit 0.
- `command -v ssh` prints nothing (or `absent`).
- `git diff origin/master -- Dockerfile` is one added package name on the `alpine` stage's `apk --no-cache add` line and nothing else.

### Post-deploy (needs the image released and the nuke pin bumped)

- The five Post-Deploy criteria above, each probed by a prompt against the live `claude-interactive` pod in nuke dev.
- The clone probe must run with **no credential in the pod's environment** — that is the state this increment ships in. If the probe needs a credential, this increment is not what made it pass, and the criterion is not met.

## Constraints

- **`agent-claude` is a shared image.** `claude-interactive` runs it, and so does `claude-headless`. Adding a binary is additive and changes neither workload's behaviour, but the change must not touch `ENTRYPOINT`, `CMD`, `WORKDIR`, env, or the `agent/` tree — those are shared.
- **The final image is built `FROM` the named `alpine` stage**, not from the base `alpine` image. `git` goes on that stage's existing `apk --no-cache add` line; a new `RUN apk add` in the final stage would work but would split package installation across two places.
- **The clone path must be under `/agent`.** Claude's tool sandbox is its WorkingDirectory, which is `/agent` in this pod (a `pwd` turn returns `/agent`). A checkout anywhere else succeeds and is then **unreadable** — `Read`/`Glob`/`Bash` all refuse paths outside the sandbox. `repo-clone.sh` in the sibling agents defaults to `/agent/repos` and its header documents this exact failure. Clone into a subdirectory, never `/agent` itself: `/agent` holds the agent's own `.claude/` and `scripts/`, and repo content is untrusted.
- **No volume is required, and adding one is not this spec's business.** `/agent` is writable in the container's layer — proven on this pod 2026-10-08, when a probe wrote `/agent/00 Inbox/pod-probe.md` and the file landed. A second mount would in any case be an **executor** change, not a chart bump: the `agent` chart deliberately does not render a claim for `type: service`, and `agent-task-executor/pkg/spawner/service_reconciler.go` calls `AddVolumeMounts` exactly once, with `datadir` at the fixed default `/home/pi/.pi`. Setting the CR's `volumeMountPath` moves that single mount rather than adding one, and is production-touching. None of it is needed here.
- **Ephemeral-storage is the real sizing limit, and it is the one risk this increment carries.** The pod's `ephemeral-storage` limit is 2Gi in `values-dev.yaml`, and a clone in the writable layer counts against it. That is comfortable for a markdown vault (tens of MB) and may not be for a large source repo. This spec does not change the limit; the credential increment decides whether a shallow clone (`--depth`) or a raised limit is needed. A probe that clones a large repo and hits the limit is a real failure of this increment's usefulness even though `git` itself works — record it rather than working around it.
- **This repo's CLAUDE.md forbids coding directly** — all code changes go through the dark-factory pipeline. That is why this is a spec.
- **`ssh` must stay absent.** The credential increment is HTTPS + installation token; an SSH client arriving as a transitive dependency of `git` would widen the image beyond this spec's scope. The `git` apk package does not pull it in, and a criterion asserts this.
