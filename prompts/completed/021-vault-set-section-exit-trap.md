---
status: completed
summary: Hoisted vault-set-section.sh's temp paths to script scope behind a cleanup trap so a successful write exits 0 and cleans up on every exit path, added a process-level regression test and wired it into make precommit's new shell-test target, and recorded the fix in CHANGELOG.md
execution_id: agent-claude-vss-exit-exec-021-vault-set-section-exit-trap
dark-factory-version: v0.198.7
created: "2026-10-12T18:58:35Z"
queued: "2026-10-12T18:58:35Z"
started: "2026-10-12T20:48:22Z"
completed: "2026-10-12T20:54:35Z"
---
# Fix the vault section helper reporting failure after a successful write

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. THE DEFECT, MEASURED. Reproduced on 2026-10-12 against the shipped v0.19.1
   helper with a stubbed `curl` first on `PATH` and no network: the GET fires,
   the POST fires and prints `{"ok":true}`, and only then does the shell die with
   `tmp_src: unbound variable` and exit 1. The write path is correct; the exit
   code lies. A cluster worker told to report the helper's result verbatim
   therefore reads a correct write as a failure and may retry or fall back to a
   hand-built whole-file POST — exactly the section-destroying corruption this
   helper shipped to replace.

2. WHY A PROCESS-LEVEL TEST, NOT A FUNCTION-LEVEL ONE. `main()` cannot be called
   from a sourcing test and still reproduce the defect: the existing test turns
   `set -u` off (`set +e +u +o pipefail`) right after sourcing, so the trap's
   `"$tmp_src"` expands to empty, `rm -f ""` succeeds, and the bug disappears.
   The defect is a property of the *script process* — the trap fires at script
   exit, after `main` has returned — so the new test must invoke the helper as a
   subprocess. Requirement 2 says so explicitly.

3. THE TEMP-FILE LEAK IS WHY THE TRAP IS NOT SIMPLY DELETED. Because the trap
   dies before its `rm -f` runs, both `mktemp` files leak on every invocation —
   measured as exactly +2 files per run. Deleting the trap would clear the exit
   code and keep the leak. Substituting `${tmp_src:-}` in the existing trap does
   not fix it either: the variables are gone by then, so the guard turns the trap
   into a silent no-op and both files leak permanently. Requirement 1's hoist is
   the only one of the three candidate fixes that cleans up on every exit path.

4. REQUIREMENT 3 IS PART OF THE CHANGE, NOT AN OPTION. `CHANGELOG.md`'s v0.19.1
   bullet records that this test "is run only by hand and is not yet wired into
   `make precommit`" — a known, documented gap rather than a policy. Left
   unwired, the new regression test is dead code that cannot fail on anything,
   which defeats the point of adding it. The change is one `wildcard` line and
   one target, inserted beside the neighbouring `python-test`. The requirement
   text carries no strike-it-yourself option; if the reviewer wants the change
   scoped to the helper alone, that is a decision to make here, not an
   instruction to leave in the artifact the container executes.

5. NOT IN SCOPE. Two further misbehaviours were observed the same day and are
   deliberately NOT touched here, each its own row: `vault-write.sh` returns
   `curl` exit 22 (HTTP >= 400) after the server has already committed the write,
   and a content file whose first line repeats the target heading yields a
   duplicated heading, because `splice_section` prints the matched heading and
   then the content file verbatim. Do not "improve" the splice here.

6. EXPECT THE TREE TO COME BACK DIRTY ON FILES THIS PROMPT DOES NOT TOUCH.
   `make precommit` runs `format`, `generate` and `addlicense`, which rewrite
   `.go` files — observed here as a gofmt column re-align in `main.go` and a
   stripped license header in `mocks/mocks.go`, both produced by a preflight run
   with no source change. Those are toolchain rewrites, not this prompt's work.
   Requirement 5 tells the agent so.
-->

<summary>
- The vault section helper stops reporting failure after a write that actually succeeded.
- An agent reading the helper's exit code now sees the truth, so it no longer retries or falls back to rewriting a whole file.
- The helper's temporary files are cleaned up on every exit path, the failure paths included, not only the successful one.
- A test reproduces the false failure on the shipped code and passes on the fixed code, so the defect cannot return unnoticed.
- That test runs automatically in the project's own precommit chain instead of only by hand.
- Nothing else about the helper changes: the section splice, the HTTP calls, the headings and the messages are all untouched.
- A changelog entry records the fix, so the release that follows carries it.
</summary>

<objective>
Make `agent/scripts/vault-set-section.sh` exit 0 when its write succeeds, so that a cluster worker told to report the helper's result verbatim stops reading a correct write as a failure — today's false failure signal points a worker at exactly the whole-file rewrite this helper was shipped to replace.
</objective>

<context>
Read these files before editing:

- `agent/scripts/vault-set-section.sh` — the whole file (114 lines). It is the only production file this prompt changes. `set -e -u -o pipefail` is line 6.
- `agent/.claude/CLAUDE.md` § Vault — documents these three helpers and the read-modify-write contract they implement. Read it to confirm the contract this change must leave unchanged; no edit to it is in scope.
- `agent/scripts/vault-set-section_test.sh` — the whole file. It sources the helper and exercises `splice_section` against temp fixtures; it never invokes the script as a process, so it never reaches `main`'s exit path.
- `Makefile.precommit` — the `precommit` target and its prerequisite list (line 5), plus the `python-check` / `python-test` pair around lines 13–35. Requirement 3 follows their shape.
- `CHANGELOG.md` — line 1 is `# Changelog`; the first `## ` heading is `## v0.19.1` at line 9. There is **no** `## Unreleased` section today; requirement 4 creates it.
- `docs/dod.md` — the Definition of Done this change is graded against.

The defect, stated at source. In `main()` (lines 90–94):

```
local tmp_src tmp_out

tmp_src=$(mktemp)
tmp_out=$(mktemp)
trap 'rm -f "$tmp_src" "$tmp_out"' EXIT
```

The trap is armed inside `main`, but an `EXIT` trap fires when the **script** exits — which is after `main` has returned and its `local` declarations have gone out of scope. The script runs under `set -u`, so when the trap expands `"$tmp_src"` the variable is unset, the shell raises `tmp_src: unbound variable`, and the process exits 1. This happens *after* the POST has already succeeded, which is why stdout carries the gateway's `{"ok":true}` and the exit code still reads as failure. The same failure means the trap's `rm -f` never runs, so both `mktemp` files leak on every invocation.

**The defect is success-path only, and that is the mechanism's signature.** When the POST fails, `set -e` exits the shell from *inside* `main`, so `main`'s locals are still live and the trap resolves them normally — measured against the shipped helper on a POST-failure path: exit `22`, **no** stderr, **no** leaked files. The trap breaks only on the path where `main` returns normally, because that is the only path that destroys its locals before the trap fires. This is also why the fix must keep the trap at script scope: a "fix" that moved cleanup to the end of `main` would clean up on success and leak on every failure.

Measured evidence (2026-10-12, stubbed `curl` first on `PATH`, no network), which the executing agent should treat as the baseline to restore:

- exit code `1`
- stdout `{"ok":true}`
- stderr `tmp_src: unbound variable`
- the stub recorded both calls: the GET, then the POST carrying the spliced body
- the temp-directory file count rose by exactly 2 across two runs

Constraints of the execution environment: the container has no Docker socket, no `kubectl` and no `dark-factory` CLI, and `.git` is masked, which empties `ROOTDIR` — pass `ROOTDIR=/workspace` on every `make` call. `bash`, `grep`, `diff`, `mktemp` and `make` are all available.
</context>

<requirements>
1. **Make the cleanup reachable from the trap, without weakening it.** In `agent/scripts/vault-set-section.sh`, stop declaring the two temporary paths `local` to `main`, so they are still in scope when the `EXIT` trap fires. Hoist them to script scope as empty strings, define a `cleanup` function that removes them, and arm the trap at top level before `main` is defined:

   ```bash
   tmp_src=""
   tmp_out=""

   cleanup() {
     rm -f "${tmp_src:-}" "${tmp_out:-}"
   }

   trap cleanup EXIT
   ```

   `main()` then **assigns** the two globals (`tmp_src=$(mktemp)`, `tmp_out=$(mktemp)`) and no longer declares them, and the `trap … EXIT` line inside `main` is removed. Keep `${tmp_src:-}` / `${tmp_out:-}` inside `cleanup` — a failure before the first `mktemp` must not turn cleanup into a second crash. Leave the `set -e -u -o pipefail` line, the `GATEWAY_SECRET` guard, the argument-count check and every `local` that is *not* a temp path exactly as they are.

   Do **not** solve this by deleting the trap (both files then leak) or by only adding `${tmp_src:-}` to the existing trap (the variables are already gone by then, so the trap becomes a silent no-op and both files still leak).

2. **Add a regression test that invokes the helper as a subprocess.** Extend `agent/scripts/vault-set-section_test.sh` with a new case, in the file's existing style (`pass` / `bad`, `fail` accumulation, a temp directory under `${tmp}`):

   - Create a stub `curl` at `${tmp}/bin/curl`, executable, that never touches the network. It must write `{"ok":true}` to stdout for the POST and, for the GET, `cat` a fixture file you create at `${tmp}/remote.md`, so the helper's `curl … > "$tmp_src"` captures a file to splice. The stub runs as its own process, so bake that path into it (or export it and have the stub read the variable) — an unnamed fixture cannot be found. Detect the POST by scanning `"$@"` for an element equal to `POST` that immediately follows an element equal to `-X` — never by substring-matching `-X POST`, which is never a single argv element.
   - Give the stub a failure switch: when a marker file `${tmp}/fail-post` exists, the stub exits non-zero for the POST instead of printing the ok body. The third assertion below uses it.
   - Set `GATEWAY_SECRET` to a non-secret dummy value — `main` requires it, and the stub never reads it.
   - Create a private spill directory, `mkdir -p "${tmp}/spill"`, and invoke the helper **as a process**, never by sourcing: `PATH="${tmp}/bin:${PATH}" GATEWAY_SECRET=dummy TMPDIR="${tmp}/spill" bash "${here}/vault-set-section.sh" <path> <heading> <content-file>`, capturing stdout and stderr to files under `${tmp}` and reading the exit code immediately into a variable.
   - Assert, naming the case so a failure says which part broke:
     1. the exit code is `0`, stdout contains `{"ok":true}`, and stderr is empty;
     2. the run left no temporary files behind — count the entries matching `tmp.*` in `${tmp}/spill` immediately before and immediately after the invocation, and require the count to be unchanged. Pointing `TMPDIR` at that private directory is what makes the assertion real: a bare `${TMPDIR}` is empty in a container that does not set it, so the count would read `0` before and after and the assertion would pass without measuring anything, and even `${TMPDIR:-/tmp}` names a shared directory a concurrent writer can flake;
     3. the same cleanup holds on a **failure** path — create `${tmp}/fail-post`, run the helper a second time, and require that no temporary files remain even though the POST failed. This one does **not** discriminate against the shipped helper: it already cleans up on that path, for the reason in `<context>`, so assertion 3 is a regression guard on the fix rather than part of the must-fail-on-shipped evidence. Keep it anyway — a fix that moved cleanup to the end of `main` would satisfy assertions 1 and 2 and leak here.

   This case must **fail** against the shipped helper — exit `1`, stderr naming `tmp_src: unbound variable`, and a leaked-file count of `+2` — and pass against the fixed one. Do not weaken it into a check that cannot fail: a test that passes on the broken input measures nothing.

   Keep the existing `splice_section` cases and their order intact; append the new case before the final `exit "$fail"`.

3. **Run the shell test in the project's precommit chain.** `Makefile.precommit` globs only `scripts/*.py` (line 13, repo-root `scripts/`) and runs only `python3 -m unittest discover -s scripts` (line 33), so nothing executes `agent/scripts/*_test.sh` today. Add `SHELL_SCRIPTS = $(wildcard agent/scripts/*_test.sh)` beside `PYTHON_SCRIPTS` — note the directory is `agent/scripts/`, **not** the repo-root `scripts/` that `PYTHON_SCRIPTS` globs, and a glob of the wrong directory would match nothing and silently satisfy the target below — a `shell-test` target that runs each discovered `*_test.sh` with `bash` and fails the target on the first non-zero exit, and insert `shell-test` into the `precommit` prerequisite list (line 5) next to `python-test`. Follow the surrounding targets' shape — a `@`-prefixed recipe, a one-line echo naming what ran, and the repo's existing variable conventions. If `SHELL_SCRIPTS` is empty the target must succeed quietly rather than error.

4. **Record the fix in the changelog.** `CHANGELOG.md` has no `## Unreleased` section today. Add one immediately below the frozen preamble (after the `Semantic Versioning` line and its blank line) and above `## v0.19.1`, containing exactly one bullet beginning `- fix:`. Write it in the style of the neighbouring released bullets — one long sentence, why-led, naming the observable that was wrong (a successful write reporting failure) and the consequence (a worker reads it as failure and may fall back to the whole-file rewrite the helper exists to replace). Say in the same bullet that the regression test is now run by `make precommit`, superseding the v0.19.1 note that recorded it as hand-run only. Do not touch any released section.

5. **Leave files this prompt does not own exactly as you found them.** `make precommit` runs `format`, `generate` and `addlicense`, which can rewrite `.go` files with no source change behind them — a gofmt column re-align, a license header added or stripped. Any such rewrite is the toolchain's, not this change's. Do not commit it, do not revert it by hand, and do not treat it as evidence about your work; the change this prompt owns is confined to the four files named in requirements 1–4 — `agent/scripts/vault-set-section.sh`, `agent/scripts/vault-set-section_test.sh`, `Makefile.precommit` and `CHANGELOG.md`.

Before finishing: re-run `<verification>` and confirm every line passes, then walk each numbered requirement against the change and confirm it is met.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Change `splice_section`'s behaviour in no way. Its heading matching, its replace and append branches, its blank-line handling and its "refusing to guess" error are all out of scope; only the temp-file lifetime and the trap move.
- Do not change the two `curl` invocations, the header names, the base URL, the URL path encoding or any message the script prints.
- Do not touch any `.go` file, `go.mod`, `go.sum`, `Dockerfile`, `agent/scripts/vault-read.sh`, `agent/scripts/vault-write.sh` or `agent/.claude/CLAUDE.md`.
- Do not add, remove or rename any file under `agent/scripts/` other than the two named in requirements 1 and 2.
- Do not add a package, a dependency or a `go mod` operation. This change is shell and make only.
- Existing tests must still pass, and `make precommit` must pass end to end.
- No absolute or home-relative paths anywhere; every path in the change is repo-relative.
- `vault-write.sh` returning exit 22 after a successful write, and a content file that repeats its own heading producing a duplicated heading, are both known and deliberately out of scope for this prompt.
</constraints>

<verification>
ROOTDIR=/workspace make precommit

# The regression test must be wired into the chain, not left hand-run. `make shell-test`
# alone cannot prove the glob found anything — requirement 3 requires the target to
# succeed quietly on an empty SHELL_SCRIPTS — so assert the expansion separately.
# `make -n` prints the recipe with $(SHELL_SCRIPTS) expanded, so a wrong or empty glob
# prints nothing and this grep fails.
ROOTDIR=/workspace make shell-test
ROOTDIR=/workspace make -n shell-test | grep -q 'vault-set-section_test.sh'

# Direct evidence the false failure is gone. This is the probe that reproduced the
# defect; against the shipped helper it exits 1 with `tmp_src: unbound variable`.
bash agent/scripts/vault-set-section_test.sh
</verification>
