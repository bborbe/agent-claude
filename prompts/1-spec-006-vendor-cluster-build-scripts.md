---
status: draft
spec: [006-build-target-remote-build]
created: "2026-10-26T01:36:28Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. WHY A FETCH AND NOT A COPY. The container mounts only this repo, has `.git` masked and
   has no go-skeleton checkout, so the five reference files can only arrive over HTTPS.
   The spec pins `bborbe/go-skeleton` at `18866d9d6b0d6159829f7a9e825232608067fee7` for
   exactly this reason: a pinned raw URL is reproducible and cannot be redirected by a
   branch moving. AC 1's `curl | diff` loop is the proof of byte-identity, and this
   prompt makes that loop the verification rather than a nicety.

2. FAIL-LOUD, NO HAND-WRITING. The one thing this prompt must NOT do is let an agent
   author a "close enough" build-cluster.sh. The spec's whole point is that `hack/` is a
   verbatim copy, so requirement 2 says: if the fetch fails, stop and report failure with
   the pinned URL. The spec's last Failure Modes row records the operator recovery
   (copy the five files from the local checkout, re-run).

3. MODES ARE PART OF THE CONTRACT. AC 2 asserts `755` on the three shell scripts and `644`
   on the YAML and the template. `curl -o` writes `644`, so the `chmod` is explicit and
   the verification reads it back with `stat`.

4. `ls hack/ | wc -l` MUST BE 5. That is the AC's anti-stub check: a sixth file (for
   instance a hand-written `hack/update-codegen.sh`) fails it. Requirement 4 forbids
   creating one, and requirement 7 asserts the count.

5. NO OTHER FILE MOVES. `Makefile.precommit`'s `generatek8s` target already points at the
   non-existent `hack/update-codegen.sh`; the spec records that breakage as pre-existing
   and out of scope. This prompt neither fixes nor worsens it.

6. `make precommit` DOES NOT EXECUTE THE SCRIPTS. `shell-test` globs only
   `agent/scripts/*_test.sh`, so the vendored scripts are checked by `bash -n` here and
   by the operator rung of the spec — never by precommit. `trivy fs` (vuln + secret) does
   scan `hack/dockerconfig.json.tpl`; it names a TeamVault lookup key and holds no value,
   so it is clean.
-->

<summary>
- The repository gains the five cluster-build files the reference implementation ships, under a new top-level directory.
- Every file is a byte-identical copy of the pinned upstream reference at a fixed commit, so the copy can be re-proven at any time with a diff.
- The three shell scripts are executable and the manifest and credential template are not, matching the reference's modes.
- Nothing under the new directory is edited, added to, or "improved" locally: the copy is verbatim by design, which is what makes it reusable for the next repository.
- No other file in the repository changes: no Makefile wiring yet, no image change, no documentation change.
- A build cannot yet be dispatched to a cluster, because nothing calls these scripts — that wiring is the next prompt.
- The repository's full pre-commit suite still passes with the new files in the tree.
</summary>

<objective>
Vendor the five cluster-build files from `bborbe/go-skeleton` at commit `18866d9d6b0d6159829f7a9e825232608067fee7` into a new `hack/` directory at the repository root — byte-identical, with `755` on the three shell scripts and `644` on the YAML and the template — so the next prompt can wire a cluster build target against scripts this repository did not author.
</objective>

<context>
Read `docs/dod.md` first — it is this repository's declared `validationPrompt` and the Definition of Done you are graded against. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `autoRelease: false`, `validationPrompt: docs/dod.md`; the effective config resolves `validationCommand: make precommit`.

This prompt is the first of five for spec `006-build-target-remote-build`. Read the spec at `specs/in-progress/006-build-target-remote-build.md` — particularly Constraints, Failure Modes and Acceptance Criteria 1 and 2 — because you have no other memory of it. This prompt does not depend on the other four prompts of this spec; it stands alone.

Current state, verified against the working tree:

- There is **no** `hack/` directory at the repository root. `ls hack` fails with `No such file or directory`.
- `Makefile.precommit` line 73 is `bash hack/update-codegen.sh` under the `generatek8s` target. That script does not exist and this prompt does NOT create it. `generatek8s` is not part of `precommit`, so nothing in the gate calls it.
- `Makefile.docker` today runs `DOCKER_BUILDKIT=1 docker build --pull --no-cache --rm=true --platform=linux/amd64 …` on the invoking machine. This prompt does not touch it.
- `.gitignore` does not ignore `hack/`, so the new files are tracked. `.dockerignore` contains only `/data`, so `hack/` joins the image build context — that is expected and harmless (the Dockerfile copies `.` into the build stage only).
- `make precommit` runs `ensure format generate python-check python-test shell-test test check addlicense`. `shell-test` globs only `agent/scripts/*_test.sh`, so the vendored scripts are not executed by the gate. `addlicense` touches only `*.go` files, so the new files need no license header.

The reference tree, as fetched at the pinned commit (this is what the files contain — use it only to confirm the fetch landed real content, never to re-author a file):

- `hack/build-cluster.sh` — validates `REF`, resolves the target through `hack/cluster-alias.sh`, refuses to apply when a Job of the derived name exists, renders `hack/build-job.yaml` through `sed`, applies it, waits for success or failure, and relays digest lines from the build log. It reads the environment variables `BUILD_TARGET`, `NAMESPACE`, `TIMEOUT`, `REPO_URL`, `REF`, `TAG`, `DOCKERFILE`, `IMAGE`, `PUSH`, `PUSH_SECRET`. Its missing-`REF` message is `REF is required; pass it on the make command line`.
- `hack/build-cluster-bootstrap.sh` — creates the namespace and renders `hack/dockerconfig.json.tpl` into the `buildkit-push` Secret via `teamvault-cli` and one `kubectl create secret --dry-run=client | kubectl apply -f -`. It is idempotent.
- `hack/cluster-alias.sh` — maps `sun`, `nukedev`, `nukeprod` to kubeconfigs and contexts; anything else is treated as a raw context name.
- `hack/build-job.yaml` — the BuildKit Job manifest, parameterised with the double-underscore tokens `__JOB_NAME__`, `__REPO_URL__`, `__REF__`, `__TAG__`, `__DOCKERFILE__`, `__IMAGE__`, `__PUSH__`, `__PUSH_SECRET__`, `__DEADLINE_SECONDS__` and `__SOURCE_DATE_EPOCH__`, substituted by `sed` in `hack/build-cluster.sh`. They are not Go-template fields. Its init container carries `GIT_CONFIG_*` for `/src` so git's dubious-ownership guard does not trip.
- `hack/dockerconfig.json.tpl` — a Docker config JSON whose username and password are the TeamVault expressions `{{ "eqKWoq" | teamvaultUser }}` and `{{ "eqKWoq" | teamvaultPassword }}`; it names a TeamVault lookup key and holds no credential value.

The pinned base URL is `https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack/`. This container has outbound HTTPS to `raw.githubusercontent.com`; if that is untrue at run time, requirement 2's fail-loud path applies.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no edit to `bborbe/go-skeleton`; no Makefile change; no Dockerfile change; no documentation change; no `hack/update-codegen.sh`; no local hardening, comment or formatting of the vendored scripts — a repo-specific need is expressed in the Makefile variables the script reads, never in the script.

Coding guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-dockerfile-guide.md` — the `Makefile.docker` build block and the `DOCKER_REGISTRY` arg. Read it for background only: this prompt changes neither file, and where that guide's canonical block disagrees with this spec (it carries `--pull`, `--rm=true` and a wall-clock `BUILD_DATE`), the spec wins.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Create the directory and fetch the five files from the pinned commit.** From the repository root, run:
   ```bash
   mkdir -p hack
   base="https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack"
   for f in build-cluster.sh build-cluster-bootstrap.sh build-job.yaml cluster-alias.sh dockerconfig.json.tpl; do
     curl -sSfL "$base/$f" -o "hack/$f" || { echo "FETCH FAILED: $base/$f" >&2; exit 1; }
   done
   ```
   `-f` is load-bearing: without it `curl` writes an HTML error body to `hack/<file>` with exit 0 and the copy is silently corrupt. The five names are exact — no extra file, no renamed file.

2. **If any fetch fails, stop.** Report the failing URL verbatim and finish with a failed completion report. Do NOT hand-author, reconstruct, shorten, reformat, or "restore from memory" any of the five files: a locally written substitute fails the byte-identity diff that is this prompt's entire point, and it would look correct while diverging from the reference. The spec's recovery is an operator step (copy the five files from a local `go-skeleton` checkout at `18866d9d`, then re-run this prompt), not something this container can do.

3. **Set the reference modes explicitly.** `curl -o` creates files `644`, so:
   ```bash
   chmod 755 hack/build-cluster.sh hack/build-cluster-bootstrap.sh hack/cluster-alias.sh
   chmod 644 hack/build-job.yaml hack/dockerconfig.json.tpl
   ```
   Do not make the YAML or the template executable, and do not leave any shell script non-executable.

4. **Add nothing else under `hack/`.** Do NOT create `hack/update-codegen.sh`, a `README`, a `.gitkeep`, a license file, a local patch, a `.gitignore`, or a wrapper. `ls hack/ | wc -l` must print `5`. Do NOT edit the content of any fetched file, not even a comment, a trailing newline or a CRLF conversion.

5. **Change no other file.** Do NOT touch `Makefile`, `Makefile.docker`, `Makefile.precommit`, `Makefile.variables`, `Dockerfile`, `.gitignore`, `.dockerignore`, `CHANGELOG.md`, `docs/`, `go.mod`, `go.sum`, `scripts/`, `agent/` or any `.go` file. In particular do NOT wire `hack/build-cluster.sh` into `Makefile.docker` — that is the next prompt, and doing it here makes this prompt's diff unreviewable.

6. **Confirm the fetched content is the real reference, not an error page.** After the fetch, these marker strings must be present at the stated count (they are quoted from the pinned reference, so a mismatch means the fetch went wrong). Note that `build-cluster.sh` builds its `require` failure text as `"${name} is required; pass it on the make command line"`, so the literal file text is the generic tail, not the `REF`-specific sentence:
   - `grep -c 'is required; pass it on the make command line' hack/build-cluster.sh` prints `1`, and `grep -c 'require REF' hack/build-cluster.sh` prints `1`
   - `grep -c 'teamvault-cli' hack/build-cluster-bootstrap.sh` prints at least `1`
   - `grep -c 'nukeprod' hack/cluster-alias.sh` prints at least `1`
   - `grep -c 'GIT_CONFIG_COUNT' hack/build-job.yaml` prints at least `1`
   - `grep -c 'teamvaultPassword' hack/dockerconfig.json.tpl` prints `1`
   If any marker is absent, treat it exactly as requirement 2's fetch failure: stop, do not repair by hand.

7. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) `ls hack/ | wc -l` prints `5`; (ii) the `curl | diff` loop exits 0 for all five files; (iii) `stat -c '%a %n' hack/*` prints `755` for the three shell scripts and `644` for the YAML and the template; (iv) `bash -n` exits 0 for all three shell scripts; (v) the only files added are the five under `hack/`, and no tracked file is modified.
</requirements>

<constraints>
- **`hack/` is a verbatim copy of `bborbe/go-skeleton` at `18866d9d6b0d6159829f7a9e825232608067fee7`.** No local edit of any kind. A repo-specific need is expressed in the Makefile variables the script reads, never in the script — that is what keeps this spec reusable for the next repo.
- **The pinned commit is what makes the copy reproducible.** Do not substitute `master`, a tag, or a newer commit, and do not "update" the files to whatever is current upstream.
- **The push credential is named, never inlined.** `hack/dockerconfig.json.tpl` names a TeamVault lookup key; nothing in this prompt resolves it, prints it, or writes a value anywhere. Do not add a credential, a token, a kubeconfig or a `.docker/config.json` to the tree.
- **Do NOT create `hack/update-codegen.sh`.** `make generatek8s` invokes it and it does not exist; that breakage predates this change and the spec records it as out of scope. Creating a stub would make `generatek8s` look fixed while doing nothing.
- **Do NOT touch `Makefile.docker`.** The dispatch on the build target is prompt 2 of this spec.
- **Do NOT touch `Dockerfile`.** The vendored-tree and base-registry change is prompt 4 of this spec.
- **Do NOT touch `CHANGELOG.md` or `docs/`.** Prompt 5 of this spec records the change; this prompt is one of five and must not claim the feature.
- Do NOT commit — dark-factory handles git. Do not run any `git` command: `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT run `docker`, `kubectl`, `make build`, `make buca`, `scripts/*.sh` or `hack/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling. The vendored scripts are copied and syntax-checked here; they are executed only on the operator side.
- Existing tests must still pass.
- This prompt adds no Go code. If you find yourself tempted to write one, don't — `github.com/bborbe/errors` and the Go patterns do not apply here.
</constraints>

<verification>
Run each command from the repository root and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`.

A note on reading the results: absence checks are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `ls hack/ | wc -l` — prints `5`.
2. `ls hack/` — prints exactly `build-cluster-bootstrap.sh`, `build-cluster.sh`, `build-job.yaml`, `cluster-alias.sh`, `dockerconfig.json.tpl`, one per line, in any order.
3. The byte-identity loop (this is AC 1 of the spec, verbatim):
   ```bash
   for f in build-cluster.sh build-cluster-bootstrap.sh build-job.yaml cluster-alias.sh dockerconfig.json.tpl; do
     curl -sSfL "https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack/$f" | diff - "hack/$f" || exit 1
   done
   ```
   exits 0 and prints nothing. Any output line is a local edit or a truncated fetch.
4. `stat -c '%a %n' hack/*` — prints `755 hack/build-cluster-bootstrap.sh`, `755 hack/build-cluster.sh`, `755 hack/cluster-alias.sh`, `644 hack/build-job.yaml`, `644 hack/dockerconfig.json.tpl` (order is glob order).
5. `bash -n hack/build-cluster.sh && bash -n hack/build-cluster-bootstrap.sh && bash -n hack/cluster-alias.sh && echo parse-ok` — prints `parse-ok`.
6. `test -x hack/build-cluster.sh && test -x hack/build-cluster-bootstrap.sh && test -x hack/cluster-alias.sh && echo executable` — prints `executable`.
7. `grep -c 'is required; pass it on the make command line' hack/build-cluster.sh` — prints `1`; `grep -c 'require REF' hack/build-cluster.sh` — prints `1` (the `REF` precondition is the script's own, and its message is assembled at run time, so the literal file text is the generic tail).
8. `grep -c 'teamvault-cli' hack/build-cluster-bootstrap.sh` — prints at least `1`.
9. `grep -c 'nukeprod' hack/cluster-alias.sh` — prints at least `1`.
10. `grep -c 'GIT_CONFIG_COUNT' hack/build-job.yaml` — prints at least `1`.
11. `grep -c 'teamvaultPassword' hack/dockerconfig.json.tpl` — prints `1`.
12. `! test -f hack/update-codegen.sh` — exits 0 (the out-of-scope script was not stubbed into existence).
13. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|"(password|auth)"[[:space:]]*:[[:space:]]*"[A-Za-z0-9+/=]{16,}"' hack/*` — exits 0 (no credential value in any vendored file; the template's `password` value is a `{{ "eqKWoq" | teamvaultPassword }}` expression, not a literal).
14. `! grep -q 'hack/build-cluster.sh' Makefile.docker` — exits 0 (this prompt did not wire the dispatch; that is prompt 2).
15. `grep -c 'docker build' Makefile.docker` — prints at least `1` and `grep -c 'hack/' Makefile.docker` — prints `0` (the existing build block is untouched).
16. `head -1 CHANGELOG.md` — prints `# Changelog`; `grep -c '^## Unreleased' CHANGELOG.md` — prints `0` (prompt 5 owns the changelog entry; this prompt does not create one).
17. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command and the exit code the completion report carries. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–5.
</verification>

<success_criteria>
- A new `hack/` directory at the repository root holds exactly five files: `build-cluster.sh`, `build-cluster-bootstrap.sh`, `build-job.yaml`, `cluster-alias.sh`, `dockerconfig.json.tpl`.
- Each file is byte-identical to `https://raw.githubusercontent.com/bborbe/go-skeleton/18866d9d6b0d6159829f7a9e825232608067fee7/hack/<file>`, proven by the `curl | diff` loop exiting 0.
- The three shell scripts are mode `755`; the YAML and the template are mode `644`; `bash -n` exits 0 on all three shell scripts.
- The five marker strings in requirement 6 are present, so the fetched content is the reference implementation rather than an error page.
- `hack/update-codegen.sh` does not exist and nothing else sits under `hack/`.
- No file outside `hack/` is created, modified or deleted; in particular `Makefile.docker`, `Dockerfile`, `CHANGELOG.md` and `docs/` are untouched.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>

<completion_report>
End your run with the dark-factory completion report block:
- `"status": "success"` only if `ROOTDIR=/workspace make precommit` exited 0 and every verification command printed the stated result.
- `"status": "failed"` if any requirement is unmet, if any fetch failed, or if any verification command did not print the stated result — including a non-zero `make precommit`, with no exceptions and no "pre-existing failure" rationalisation.
- Include `"verification": {"command": "ROOTDIR=/workspace make precommit", "exitCode": <N>}` with the actual exit code.

Then append an `## Improvements` section, one bullet per observation, each tagged `[PROMPT]`, `[GUIDE]` or `[GLOBAL]`. If there is genuinely nothing to suggest, write `- None`.
</completion_report>
