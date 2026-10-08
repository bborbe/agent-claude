---
status: prompted
approved: "2026-10-08T22:28:59Z"
generating: "2026-10-08T22:29:59Z"
prompted: "2026-10-08T22:37:21Z"
branch: dark-factory/add-git-to-agent-claude-image
---

## Summary

- The `agent-claude` image gains the `git` binary, so a `claude-interactive` pod can clone a repository into its own tool sandbox and read the code.
- The change is the binary only. No credential, no authentication, no push, no helper script — those are the next increment and are named under Non-goals.
- On its own this makes the pod able to clone and read **public** repositories, which is what [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]] needs for the read half of its SC2 against `bborbe/vault-ui`.
- `git` lands on the existing `apk --no-cache add` line of the `alpine` build stage, which the final image is already built `FROM`, so nothing else in the image changes shape.
- `ssh` is deliberately not added: the chosen credential is a GitHub App installation token over HTTPS, which needs no SSH client.

## Problem

A `claude-interactive` pod cannot read any repository, and the reason is narrower than "the pod has no repo": the image ships no `git` at all. Measured 2026-10-08, the live pod runs `bborbe/agent-claude:v0.12.1`, and at that tag the `alpine` stage — the stage the final image is built `FROM` — installs exactly `ca-certificates curl bash nodejs npm python3` and then `apk del npm`; there is no `git` and no `openssh-client`, and the one package-installing `RUN` in the `build` stage is discarded because only `/main` is copied across. The consequence is already recorded on [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]]: a replayed `/vault-cli:work-on-task` prompt returned `Vault-UI codebase not found in the agent directory`, because there is no checkout and no tool with which to make one. That task's SC2 cannot be attempted by any mechanism while `git` is absent — a credential would not help, because there is nothing to authenticate with.

## Goal

After this work, a `claude-interactive` pod can clone a named public repository into a directory its own `Read`/`Grep`/`Bash` tools can reach, and print a named file's real content from that clone — with no credential involved and no other capability of the image changed.

## Non-goals

- **Any credential.** No GitHub App, no installation token, no PEM, no deploy key, no SSH client, no credential helper, no `~/.ssh`, no `GITHUB_APP_*` env. Authentication is a separate increment with its own spec, gated on a GitHub App that does not exist yet.
- **Push.** This increment clones and reads. Writing to a remote is the credential increment's job.
- **Changing the `claude-interactive` wiring.** No Config CR change, no new Secret, no new env var, no chart change, no executor change, no manifest edit.
- **Adding `ssh` or `openssh-client`.** The chosen credential is HTTPS-only.
- **Changing the agent's guardrails.** No rule text changes here. **Assumption to confirm at implementation time:** the pod agent's guardrail permits public internet *"for documentation and research"* and separately forbids package installation and shell escapes. That a clone of a public repo falls under the permitted clause is an **interpretation of that text, not a quotation of it** — the operator ruled 2026-10-08 that no guardrail change is needed, and this spec proceeds on that ruling. If the interpretation is ever rejected, the guardrail text is its own change, not a silent edit here.
- **Making the checkout persistent.** The clone lands in the container's writable layer by design. Persistence is not attempted.
- **Changing `github.com/bborbe/agent`** — no library surface is added.
- **Teaching the agent when to clone.** Telling the pod when to clone and what to do with the checkout is the credential increment's job, where the push policy is also decided. This increment only makes the capability exist.

## Acceptance Criteria

- [ ] **Container:** the built image contains `git` and reports its version. Evidence: `docker run --rm --entrypoint git <image> --version` exits 0 and prints a `git version …` string. (No `deploy_check` — checkable against the built image before any cluster is touched.)
- [ ] **Container:** everything the image had before still resolves. Evidence: `docker run --rm --entrypoint sh <image> -c 'bash --version >/dev/null && node --version && python3 --version && echo rc=0'` prints `rc=0`.
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
- [ ] **Post-Deploy (Rung-2):** the clone is writable — this increment must NOT copy `repo-clone.sh`'s read-only hardening, because the credential increment must push from this same checkout. Evidence: after the clone, a file created inside the worktree can be removed again.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] **Post-Deploy (Rung-2):** the pod is still `1/1 Running` with `0 restarts` after the probes, and a second sequential prompt is answered, proving the session is still held across requests. Evidence: `kubectlnukedev -n dev get pod claude-interactive-0` reads `1/1 Running` and `0` restarts, and the second prompt's reply references the first.
  - `deploy_check:` `kubectlnukedev -n dev get statefulset claude-interactive -o jsonpath='{.spec.template.spec.containers[0].image}' | awk -F: '{print $NF}'`
  - `deploy_target:` `v0.13.0`
- [ ] No credential value appears anywhere this increment touches. Evidence: `git diff origin/master` contains no `GITHUB_APP`, `PEM`, `x-access-token`, `deploy-key` or `ssh` string, and `kubectlnukedev -n dev logs claude-interactive-0 | grep -ci 'x-access-token\|Authorization: Bearer'` prints `0`.

## Verification

### Container-executable (runs inside the YOLO container at prompt time)

- `git diff origin/master -- Dockerfile` is one added package name on the `alpine` stage's `apk --no-cache add` line and nothing else.

This is the only check here that needs no Docker socket. The image probes below are deliberately **not** in this rung: they run `docker`, which the container does not have, and a prompt-creator that inherits them as container-executable would either fail on them or silently drop them.

### Operator-executable (runs on the host — needs `docker`, then the cluster)

- `docker run --rm --entrypoint git <image> --version` exits 0 and prints a version.
- `docker run --rm --entrypoint sh <image> -c 'bash --version >/dev/null && node --version && python3 --version && echo rc=0'` prints `rc=0`.
- `docker run --rm --entrypoint sh <image> -c 'command -v ssh || echo absent'` prints `absent`.
- Once the image is released and the nuke pin bumped: the five Post-Deploy criteria above, each probed by a prompt against the live `claude-interactive` pod in nuke dev.
- The clone probe must run with **no credential in the pod's environment** — that is the state this increment ships in. If the probe needs a credential, this increment is not what made it pass, and the criterion is not met.

## Desired Behavior

1. The image contains a working `git` binary, and its version is printable from inside the container.
2. Everything the image did before still works: `bash`, `node`, the Claude Code CLI, `python3`, the pod-side attention poster, and the existing `ENTRYPOINT`.
3. From inside a `claude-interactive` pod, a clone of a public repository into a path under `/agent` completes without a credential.
4. The agent's own tools — `Read`, `Grep`, `Bash` — can read files inside that clone. The clone being on disk is not sufficient; it must be inside the tool sandbox.
5. The clone is writable, so a later increment can commit and push from the same checkout.
6. No SSH client is present, and no credential of any kind is introduced.
7. The pod keeps serving: it stays `1/1 Running` with `0` restarts, and a second sequential prompt is answered.

## Constraints

- **`agent-claude` is a shared image.** `claude-interactive` runs it, and so does `claude-headless`. Adding a binary is additive and changes neither workload's behaviour, but the change must not touch `ENTRYPOINT`, `CMD`, `WORKDIR`, env, or the `agent/` tree — those are shared.
- **The final image is built `FROM` the named `alpine` stage**, not from the base `alpine` image. `git` goes on that stage's existing `apk --no-cache add` line; a new `RUN apk add` in the final stage would work but would split package installation across two places.
- **The clone path must be under `/agent`.** Claude's tool sandbox is its WorkingDirectory, which is `/agent` in this pod. A checkout anywhere else succeeds and is then unreadable — `Read`/`Glob`/`Bash` all refuse paths outside the sandbox. Clone into a subdirectory, never `/agent` itself: `/agent` holds the agent's own `.claude/` and `scripts/`, and repo content is untrusted.
- **No volume is required, and adding one is not this spec's business.** `/agent` is writable in the container's layer — proven on this pod 2026-10-08, when a probe wrote `/agent/00 Inbox/pod-probe.md` and the file landed. A clone in the writable layer therefore needs no claim, no mount and no manifest change. (For the record, if persistence were ever wanted it is an **executor** change in `bborbe/agent`, not a chart bump, and setting the CR's `volumeMountPath` moves the pod's single existing mount rather than adding one — verified 2026-10-09 by the vault-in-pod worker. Both are other repos and out of scope here; recorded so nobody re-derives it.)
- **Ephemeral-storage is the real sizing limit, and it is the one risk this increment carries.** The pod's `ephemeral-storage` limit is 2Gi in `values-dev.yaml`, and a clone in the writable layer counts against it. That is comfortable for a markdown vault (tens of MB) and may not be for a large source repo. This spec does not change the limit.
- **This change is authored as a spec rather than hand-written.** The authority that travels with the repo is `.dark-factory.yaml`, which carries `autoGeneratePrompts: true` — the pipeline is configured here. The working checkout additionally has a `CLAUDE.md` stating the rule in words (*"Never code directly. All code changes go through the dark-factory pipeline"*), but note it is **not in the repo**: `/CLAUDE.md` is in `.gitignore`, so it is absent from a fresh worktree and a prompt-creator container mounting only this repo will never see it. Cite `.dark-factory.yaml`, not the gitignored file.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| A clone is attempted outside `/agent` | The clone succeeds on disk but `Read`/`Glob`/`Bash` refuse every path in it, so the turn wastes the clone and escalates — the exact failure `repo-clone.sh`'s header documents | Clone under `/agent/repos`; the credential increment's instructions must name the path |
| A clone exceeds the 2Gi `ephemeral-storage` limit | The write fails mid-clone, or the pod is evicted | Use a shallow clone (`--depth`); if still over, the limit is raised in `values-dev.yaml` — a separate change, out of scope here |
| The pod restarts mid-task | The writable-layer clone is gone | Re-clone lazily per task; nothing depends on persistence, by design |
| `git` pulls in an SSH client transitively | The image widens beyond this spec's scope | A criterion asserts `ssh` is absent; if it appears, narrow the package set rather than accept it silently |
| The added package breaks the image build | The build fails and nothing deploys | The change is one token on an existing line; revert it |
| A repo's content shadows the agent's own files | The agent could read attacker-controlled content as its own config or scripts | Clone into `/agent/repos/<owner>/<repo>`, never into `/agent` itself |

## Security / Abuse

- **Untrusted content enters a directory the agent reads.** A cloned repo's files are attacker-controlled if the repo is. The clone lives under `/agent/repos/`, never at `/agent` itself, so repo content cannot shadow the agent's own `.claude/` or `scripts/`. This is the constraint `repo-clone.sh` already encodes for the sibling agents.
- **No credential exists in this increment**, so a malicious repo has nothing to exfiltrate and a compromised clone cannot reach any remote. This is a property of the increment being binary-only, and it is why the credential work is deliberately separate.
- **Public repositories only.** Nothing here authenticates, so nothing here can reach a private repo — including the vault.
- **The agent's existing guardrails are unchanged** and continue to forbid printing or transmitting env vars, keys and credentials.
- **No new network surface.** `github.com` over HTTPS is public internet, already permitted by the agent's guardrail; no internal address is introduced.

## Suggested Decomposition

One prompt. The change is a single package name on an existing `apk --no-cache add` line plus a comment explaining why `git` is there. Splitting it would produce a prompt whose only content is a build-config edit.

## Do-Nothing Option

Doing nothing leaves the pod unable to read any repository. That blocks [[A Cluster Worker Cannot Reach a Source Repo, So Repo Work Cannot Be Carried]] at SC2 and SC3, and through them the goal's SC5 — so no cluster worker can carry a repo-bearing task at all, which is the capability the whole goal exists to create. The cost of the change is one package name on a line that already installs five, plus one image release; the cost of not doing it is that the credential increment has nothing to authenticate with. Do-nothing is not acceptable.
