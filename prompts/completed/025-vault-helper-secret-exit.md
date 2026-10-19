---
status: completed
summary: Both vault helpers now guard GATEWAY_SECRET explicitly and exit 1 on every bash version, with a new missing-secret test case and a changelog fix entry.
execution_id: agent-claude-exec-025-vault-helper-secret-exit
dark-factory-version: v0.198.7
created: "2026-10-20T00:20:00Z"
queued: "2026-10-19T22:27:14Z"
started: "2026-10-19T22:27:19Z"
completed: "2026-10-19T22:31:34Z"
---

# Vault helpers exit non-zero when the secret is missing, on every bash

<summary>
- A vault helper that runs without its gateway secret now exits non-zero the same way on every bash version.
- The status currently depends on which bash runs the script: on bash 3.2 it prints "GATEWAY_SECRET is required" and then exits **0**, so a caller reading the exit code sees success on a request that was never sent; bash 5 already exits 1.
- That matters because this repo's preflight runs `make precommit` **on the host**, which on macOS is bash 3.2 — so the host baseline is red today, while the same suite is green inside the execution container.
- Two helpers are affected: the one that sets a section and the one that completes a task row.
- The substring `GATEWAY_SECRET is required` is unchanged, so callers and tests that grep for it keep working.
- The check now exits 1 explicitly instead of depending on a shell side effect.
- A test case is added for the section helper, which had no coverage of this path.
- A changelog entry records the fix.
- No behaviour changes on the success path, and no new file, package or credential.
</summary>

<objective>
Make a missing `GATEWAY_SECRET` exit non-zero on every bash version in `agent/scripts/vault-set-section.sh` and `agent/scripts/vault-complete.sh`, so a caller that branches on the exit code cannot mistake "the secret was absent, nothing was sent" for a successful write. Bash 5 already exits 1; macOS `/bin/bash` 3.2 exits 0 because the `EXIT` trap's `rm -f` supplies the final status — so today the outcome depends on which bash runs the helper, and the host-side preflight sees the failing one.
</objective>

<context>
Read `docs/dod.md` first — it is the Definition of Done you are graded against and this repository's declared `validationPrompt`. Read `.dark-factory.yaml` — `workflow: direct`, `pr: false`, `autoMerge: false`, `autoRelease: false`; the resolved `dark-factory config` also sets `preflightCommand: make precommit`, which is a default rather than a key in the file.

This is a standalone prompt: there is no spec file to read.

**The defect, precisely.** Both scripts begin `main` with `: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"` and both install an `EXIT` trap (`trap cleanup EXIT`) whose handler runs `rm -f`. The status that path produces is **bash-version-dependent**:

- **bash 3.2** (macOS `/bin/bash`, and therefore this repo's host-side preflight): the `:?` expansion prints the message, the `EXIT` trap observes status **0**, and the trap's `rm -f` becomes the script's final status — so the script exits **0**. Measured on this host:
  ```
  env -u GATEWAY_SECRET /bin/bash agent/scripts/vault-complete.sh t.md; echo $?
  # prints: ...: GATEWAY_SECRET is required
  # prints: 0
  ```
- **bash 5** (the execution container, `bborbe/claude-yolo`, GNU bash 5.2.15): the same command exits **1**.

An explicit `exit 1` inside `main` is **not** version-dependent: the trap observes 1 and the script exits 1 on both. The two sibling helpers with no trap — `vault-read.sh` and `vault-write.sh` — already exit 1 on both versions, which is why only these two need the change.

Read these files before editing:

- `agent/scripts/vault-complete.sh` — the guard is the first statement of `main`.
- `agent/scripts/vault-set-section.sh` — same guard, same position.
- `agent/scripts/vault-read.sh` and `agent/scripts/vault-write.sh` — the correct shape, for reference only; do not edit them.
- `agent/scripts/vault-complete_test.sh` — its `process: missing GATEWAY_SECRET …` case asserts a non-zero exit and greps stderr for `GATEWAY_SECRET is required`. It must pass unchanged.
- `agent/scripts/vault-set-section_test.sh` — the sibling test; requirement 6 adds a missing-secret case to it.
- `CHANGELOG.md` — line 1 is `# Changelog`; the first `## ` heading is `## Unreleased`, holding one `feat:` bullet.

### What is out of scope

No change to `vault-read.sh`, `vault-write.sh`, `main.go`, any `.go` file, `go.mod`, `go.sum`, `Dockerfile`, `Makefile*`, `.dark-factory.yaml`, or the repo-root `scripts/`. No change to the `EXIT` trap, the temp-file handling, the URLs, the headers, `--data-binary`, or any success-path behaviour. No new helper script.
</context>

<requirements>
1. **In `agent/scripts/vault-complete.sh`, replace the `:?` guard inside `main` with an explicit guard.** Replace the single line

   `: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"`

   with:

   ```
   if [ -z "${GATEWAY_SECRET:-}" ]; then
     echo "vault-complete.sh: GATEWAY_SECRET is required" >&2
     exit 1
   fi
   ```

   It must remain the **first** statement of `main`, before the argument-count check and before any `mktemp` or `curl`. The message must still contain the exact substring `GATEWAY_SECRET is required`, because `agent/scripts/vault-complete_test.sh` greps for it.

2. **Apply the same change to `agent/scripts/vault-set-section.sh`**, with the message prefixed `vault-set-section.sh:`. Same position — first statement of `main`, before the argument-count check and before any `mktemp` or `curl`.

3. **Add a `## Unreleased` changelog bullet.** The section already exists with one `feat:` bullet; add **exactly one** `fix:` bullet below it. It must state that the `:?` guard's exit status is bash-version-dependent — on macOS `/bin/bash` 3.2 the two helpers printed `GATEWAY_SECRET is required` and then exited 0 because the `EXIT` trap's `rm -f` supplied the final status, while bash 5 exits 1 — so a caller branching on the exit code could see success on a request that was never sent under bash 3.2, and the repo's host-side `make precommit` preflight runs under exactly that bash; that both helpers now guard explicitly and exit 1 on every bash version; and that the substring `GATEWAY_SECRET is required` is unchanged. Keep the wording in the style of the existing entries: one long sentence, backticked identifiers, em dashes. Do not move, delete or reorder any other section.

4. **Change nothing else in the two helpers.** Do not touch the `EXIT` trap, the `cleanup` function, the temp-file variables, the two `curl` invocations, the header variables, `set_status_completed`, `splice_section`, or any other line of either script. The one exception is the test file named in requirement 6.

5. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result. In particular confirm (i) both scripts still pass `bash -n`; (ii) with `GATEWAY_SECRET` unset each prints a message containing `GATEWAY_SECRET is required`; (iii) `bash agent/scripts/vault-complete_test.sh` exits 0 with every case `PASS`; (iv) both `*_test.sh` files pass and `make shell-test` passes; (v) reading each helper confirms the guard block is the only change.

6. **Add a missing-secret case to `agent/scripts/vault-set-section_test.sh`.** Mirror the `process: missing GATEWAY_SECRET …` case in `agent/scripts/vault-complete_test.sh` (around lines 197–207): invoke the helper as a process with `env -u GATEWAY_SECRET`, and assert the exit status is non-zero and stderr contains `GATEWAY_SECRET is required`; because bash 5 already exits 1 before the change, also assert the guard is the explicit form (`! grep -qF ': "${GATEWAY_SECRET:?' "${here}/vault-set-section.sh"`), so the case fails in the container too if the `:?` form returns. Follow the sibling test's existing `pass`/`bad` helper and `here=` resolution; do not restructure the file.
</requirements>

<constraints>
- **Do NOT commit — dark-factory handles git.** Do not run any `git` command; the container is started with `.git` masked, so every `git` invocation fails, and a failed command in `<verification>` is a false-positive pass because the executor does not check verification exit codes.
- **Never print, log, write to a file, or place in argv the value of `GATEWAY_SECRET`.** The guard tests only whether it is empty.
- **The message substring is load-bearing.** `agent/scripts/vault-complete_test.sh` greps stderr for `GATEWAY_SECRET is required`; changing the wording breaks a test that must pass unchanged.
- **Do not remove the `EXIT` trap.** It is what cleans up the temp files on every exit path; the fix is the explicit `exit 1`, not the trap's removal.
- **Do not use `exit 0` or `return 1`.** `return` from `main` under `set -e` behaves differently from `exit`, and the point of the change is a definite non-zero process status on every bash.
- **The exit status is version-dependent, so a green run is not evidence your edit failed.** Inside this container (bash 5) the missing-secret case already passes before your change. Confirm the edit itself with verification item 6, which asserts the `:?` form is gone — not with the exit status alone.
- **Existing tests must still pass.** `ROOTDIR=/workspace make precommit` must exit 0. `make precommit` runs `format`, `generate` and `addlicense`, which can rewrite `.go` files with no source change behind them — leave such a rewrite alone; do not revert it by hand and do not treat it as this prompt's work.
- **No new package, binary, symlink, alias, or credential.**
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, as every prompt in this repository does.

A note on reading the results: `grep -c` is used only where the expected count is non-zero; absence checks are written as `! grep -q`, which prints nothing and exits 0.

1. `bash -n agent/scripts/vault-complete.sh && bash -n agent/scripts/vault-set-section.sh` — exits 0 (both scripts are syntactically valid).
2. `env -u GATEWAY_SECRET bash agent/scripts/vault-complete.sh 'tasks/x.md' >/dev/null 2>&1; echo $?` — prints `1` (the missing secret is a non-zero exit). Bash 5 already prints `1` before the change, so this is a post-change assertion, not baseline evidence.
3. `env -u GATEWAY_SECRET bash agent/scripts/vault-set-section.sh 'tasks/x.md' '# A' /tmp/content.txt >/dev/null 2>&1; echo $?` — prints `1`, same caveat.
4. `out=$(env -u GATEWAY_SECRET bash agent/scripts/vault-complete.sh 'tasks/x.md' 2>&1); printf '%s' "$out" | grep -q 'GATEWAY_SECRET is required' && echo message-ok || echo message-MISSING` — prints `message-ok`.
5. `out=$(env -u GATEWAY_SECRET bash agent/scripts/vault-set-section.sh 'tasks/x.md' '# A' /tmp/content.txt 2>&1); printf '%s' "$out" | grep -q 'GATEWAY_SECRET is required' && echo message-ok || echo message-MISSING` — prints `message-ok`.
6. `! grep -qF ': "${GATEWAY_SECRET:?' agent/scripts/vault-complete.sh agent/scripts/vault-set-section.sh` — exits 0 (neither script relies on the parameter-expansion form any more). **This is the discriminating check** — it fails before the change and passes after, on every bash.
7. `grep -c 'GATEWAY_SECRET is required' agent/scripts/vault-complete.sh agent/scripts/vault-set-section.sh` — prints `1` for each file.
8. `grep -c 'GATEWAY_SECRET is required' agent/scripts/vault-set-section_test.sh` — prints at least `1` (the new test case asserts the message).
9. `bash agent/scripts/vault-complete_test.sh` — exits 0 and prints a `PASS` line for every case, including `process: missing GATEWAY_SECRET exits non-zero with the required message`.
10. `bash agent/scripts/vault-set-section_test.sh` — exits 0, including the new missing-secret case (the sibling test sources the script to test `splice_section` and sets `GATEWAY_SECRET=dummy` only for its existing process-level cases).
11. `ROOTDIR=/workspace make shell-test` — exits 0 (the `agent/scripts/*_test.sh` glob runs clean).
12. `head -1 CHANGELOG.md` — prints `# Changelog` (the frozen preamble is intact).
13. `grep -c '^## Unreleased' CHANGELOG.md` — prints `1`.
14. `awk '/^## Unreleased/{f=1;next} /^## /{f=0} f && /^- /{n++} END{print n+0}' CHANGELOG.md` — prints `2` (the existing `feat:` bullet plus the new `fix:` bullet).
15. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY|X-Gateway-Secret: [^$]' agent/scripts/vault-complete.sh agent/scripts/vault-set-section.sh agent/scripts/vault-set-section_test.sh CHANGELOG.md` — exits 0 (no credential value and no literal secret).
16. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared validation command. If it fails for a reason unrelated to this change, report the failing target and its output verbatim rather than editing anything outside requirements 1–3 and 6.
</verification>

<success_criteria>
- `agent/scripts/vault-complete.sh` and `agent/scripts/vault-set-section.sh` each guard `GATEWAY_SECRET` explicitly as the first statement of `main` and exit `1` when it is unset or empty.
- Neither script uses the `: "${GATEWAY_SECRET:?…}"` form any more, so the exit status no longer depends on the bash version.
- Both still print a message containing `GATEWAY_SECRET is required` on that path.
- `agent/scripts/vault-set-section_test.sh` gains a missing-secret case asserting a non-zero exit and the message, and still passes.
- `agent/scripts/vault-complete_test.sh` passes every case unchanged.
- `make shell-test` exits 0.
- `CHANGELOG.md` gains exactly one `fix:` bullet under the existing `## Unreleased`, and no other section changes.
- The `EXIT` trap, temp-file handling, URLs, headers and success paths of both helpers are unchanged; the only diff in each is the guard block.
- No credential value appears in any file this change touches.
</success_criteria>
