---
spec: ["004-github-app-token-mint"]
status: draft
created: "2026-10-09T17:03:42Z"
---

<!--
REVIEWER NOTES — for the human reviewer, NOT instructions to the executing agent.

1. LANGUAGE DECISION — the helper is PYTHON 3, not bash and not Go.
   - The spec's own Summary says "No new binary, no `gh`", so a compiled Go helper is
     out by construction; the spec's Constraint "No new package … `openssl`, `git` and
     `python3` are already in the image" names python3 as an intended ingredient.
   - bash was rejected on a hard requirement, not on taste: the mint must POST the
     signed JWT to GitHub. With `curl` the JWT lands in argv (`-H "Authorization: Bearer
     <jwt>"`), and `GITHUB_APP_PEM` is a long-lived credential, so any argv-visible
     derivative of it is a defect. Python's stdlib `urllib.request` sends the same
     request with the JWT in the process's memory only, and `openssl dgst` reads the
     signing input from stdin, so nothing credential-adjacent ever touches argv.
   - Python also gets the base64/JSON work for free from the stdlib, which is the part
     bash does worst.
   - Verified in-container before writing: `openssl genrsa` + `openssl dgst -sha256
     -sign` reading stdin + `openssl dgst -sha256 -verify` round-trips; `b64decode` with
     whitespace stripped and `validate=True` round-trips a 76-column-wrapped base64 blob
     and rejects `!!!not-base64!!!`; the base64url of `{"alg":"RS256","typ":"JWT"}` is
     `eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9`.

2. TEST STRATEGY — a Python `unittest` suite plus ONE new make target (`python-test`),
   wired into the existing `precommit` chain. This is the deliberate part of this
   prompt; the alternatives were considered and rejected:
   - `make test` is Go-only (`go test -mod=mod … $(go list ./... | grep -v /vendor/)`),
     so a Python test cannot ride the existing `test` target.
   - A Go test that execs the script was rejected because the happy path needs a seam
     to intercept the HTTPS POST to api.github.com. A Go test can only get one by
     adding a production env knob for the API base URL — which would let anything that
     can set the helper's environment redirect the minted JWT to an arbitrary host. The
     Python test instead monkeypatches the module-level `API_BASE` constant and the
     module-level `post_json` function, so the seam exists ONLY inside the test process
     and production has no override at all. Verification check 11 asserts that absence.
   - The repo already has the precedent: `Makefile.precommit` carries a `python-check`
     target (added for the vendored attention poster) whose comment explains exactly
     this "nothing here parsed scripts/*.py before this" reasoning. `python-test` is the
     same shape one step further.
   - `Makefile.precommit` is therefore edited by THIS prompt, and by no other prompt in
     the spec.

3. CHANGELOG OWNERSHIP — this prompt does NOT touch `CHANGELOG.md`. The single `feat:`
   bullet for this capability is written by sibling prompt
   `018-spec-004-install-and-wire-credential-helper.md`. dark-factory appends a generic
   "Update CHANGELOG.md" footer to every prompt; for this prompt that footer CONTRADICTS
   the scope below. Follow the body: do not edit `CHANGELOG.md` here, or the capability
   gets three changelog bullets for one feature.

4. NO SCENARIO PROMPT. The spec's Suggested Decomposition states this explicitly and the
   four-condition test agrees: the behaviour that matters (a real installation token
   authenticating a real `git push` against a real repo) is reachable only from a live
   pod and is owned by the spec's Post-Deploy (Rung-2) AC. The repo has no `scenarios/`
   directory. Unit tests cannot mint against GitHub, and the spec says so.

5. HIDE-GIT — `.dark-factory.yaml` sets `workflow: direct` and the daemon log records
   `hideGit=true hideGitSource=arg`, so `.git` is masked and every `git` invocation
   fails. No `git` command appears in `<verification>`, and `ROOTDIR=/workspace` is
   passed on every `make` call because `Makefile.variables` resolves `ROOTDIR` from
   `git rev-parse --show-toplevel`, which returns empty under a masked `.git`.

6. OPERATOR RUNG IS NOT CARRIED. The spec's Acceptance Criteria 1 and 3 (the executable
   resolves in a built image; a real mint against the dev App returns a token GitHub
   accepts) need Docker and a live App, neither of which this container has. They stay on
   the spec's "Operator-executable" rung. The functional checks in `<verification>`
   exercise the helper's loud-failure paths for real, because `python3` IS in this
   container — those are genuine end-to-end runs of the script, not greps.

7. FAILURE-MODE MAPPING (spec's Failure Modes table → requirement):
   - PEM unset/empty → req 5 (a), exit 1, stderr names `GITHUB_APP_PEM`, no `password=`
   - PEM present but not base64-decodable → req 5 (b), exit 1 naming the decode step
   - JWT rejected by GitHub (401) → req 8, exit 2, distinguished from transport failure
   - Installation does not cover the repo → req 8 note: the mint succeeds (it is
     per-installation, not per-repo) and the SERVER refuses the git operation with
     403/404. The helper does not and must not try to classify this.
   - Throttled mint (403 + rate-limit headers) → req 8, exit 3, named as a throttle
   - Network egress blocked → req 8, exit 4, transport error surfaced
   - Token expires mid-operation → req 4: nothing is cached, every invocation mints
   - Two workers mint concurrently → req 4: no shared state, no lock, no cache file
   Every row is addressed.

8. SECURITY MAPPING (spec's Security / Abuse → requirement):
   - "The helper must not echo the token" → req 9, plus verification 5, 6 and 10 (the
     failure paths assert empty/credential-free stdout) and 14 (no credential literal)
   - "takes no argument from the model that selects an App or installation" → req 2
     (the only identity inputs are the three env vars; the API base URL is a module
     constant, never read from the environment) + verification 12's absence check
   - "any diagnostic path that would print it … is a defect" → req 9

9. JUDGEMENT CALL — THE EXIT-CODE TAXONOMY (req 8). The spec never names an exit code; it
   says "non-zero" and requires the DIAGNOSTIC to distinguish the classes ("exits non-zero,
   distinguishing a rejected JWT from a network failure"). Requirement 8 pins a five-value
   taxonomy (1 local input, 2 JWT rejected, 3 throttled, 4 transport, 5 other API error) so
   that "distinguishing" is mechanically assertable rather than a matter of reading prose,
   and so a caller of the `token` subcommand can branch on the class. This is the one place
   this prompt goes beyond the spec's literal text. If the reviewer prefers a uniform exit
   1 with distinct messages only, requirements 8 and 12 (i) and the `rc=` checks in
   verification 5, 6, 9 and 10 are the places to relax — nothing else depends on the
   numbers.
-->

<summary>
- A pod can turn a GitHub App credential into a short-lived installation token, using only tools the image already ships.
- The token is produced on demand and never written to disk: no cache file, no credentials file, no config entry.
- `git` can ask for the credential through its own credential-helper protocol, so a clone or push authenticates without a password ever appearing on a command line.
- A caller that needs the token outside `git` can ask for the bare token directly.
- Every way the mint can go wrong fails loudly: a missing or malformed credential, a rejected request, a throttled request and a network failure each exit non-zero and say on stderr which input was at fault.
- On any failure the helper writes nothing that `git` could mistake for a credential, so `git` can never silently fall back to an anonymous request that appears to work against a public repo and fails against a private one.
- The credential itself is a base64-encoded key, exactly as the deployment delivers it, and the helper decodes it rather than assuming a raw key.
- A suite of automated tests exercises the credential-helper protocol, the key decoding, the token construction and every failure path, and runs as part of the repository's normal checks.
- No new package, no new Go dependency and no new image binary are introduced; nothing else in the repository changes.
- The changelog entry for this capability is written by a sibling prompt, not this one.
</summary>

<objective>
Add `scripts/git_credential_github_app.py` — a standalone Python 3 executable that mints a GitHub App installation token from `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM`, serves the git credential-helper protocol on `get` / `store` / `erase` and prints a bare token on `token`, failing loudly and writing no credential on every error path — plus its `unittest` suite and the one make target that runs it, so that the increment which wires the credential into the image has an executable whose contract is already proven. This prompt adds the helper and its tests only; installing it into the image and pointing `git` at it is the sibling prompt `018`.
</objective>

<context>
This repository has no root `CLAUDE.md` in a fresh worktree (`/CLAUDE.md` is in `.gitignore`), so do not look for one. Read `docs/dod.md` — the Definition of Done you are graded against, and the repository's declared `validationPrompt`. Read `.dark-factory.yaml` — it is the authority that travels with the repo (`workflow: direct`, `autoGeneratePrompts: true`, `autoRelease: false`).

Read these files before writing:

- `Makefile.precommit` — read the whole file. It is the file you edit. Note the `python-check` target and its comment block: it is the existing precedent for validating a Python file in this Go repository, and the `precommit:` recipe line is where `python-test` is added.
- `scripts/pod-attention.py` — read the first ~60 lines only. It is the only other first-party-ish Python in the tree and shows the house style (module docstring, stdlib imports, `if __name__ == "__main__":` guard). Do NOT copy its logic; it solves an unrelated problem.
- `go.mod` — for context only. The module is `github.com/bborbe/agent-claude`, Go 1.27.1. This prompt adds no Go code and touches neither `go.mod` nor `go.sum`.
- `docs/dod.md` — the Definition of Done.

Repository facts you must not violate (verified against the working tree):

- `Makefile.precommit` line 3 is `default: precommit`; the recipe is `precommit: ensure format generate python-check test check addlicense`.
- `PYTHON_SCRIPTS = $(wildcard scripts/*.py)` and the `python-check` target runs `python3 -c "import ast,sys; [ast.parse(...) for f in sys.argv[1:]]" $(PYTHON_SCRIPTS)`. Any `scripts/*.py` you add is therefore syntax-checked automatically — including the test file.
- `make test` is `go test -mod=mod -p=$${GO_TEST_PARALLEL:-1} -cover $(TESTFLAGS_RACE) $(shell go list -mod=mod ./... | grep -v /vendor/)` — it runs Go tests only. A Python test cannot ride it.
- `Makefile.variables` is `ROOTDIR ?= $(shell git rev-parse --show-toplevel)`. Under this container's masked `.git` that resolves empty, so every `make` invocation in `<verification>` passes `ROOTDIR=/workspace`.
- `scripts/` today holds exactly `pod-attention.py` and `answered-attribution.py`, both vendored verbatim from `bborbe/claude-supervisor` and copied into the image by an explicit `COPY` in `Dockerfile`. Adding a first-party script beside them is intended; nothing copies `scripts/` wholesale.

Why this change: `claude-interactive` was given a GitHub App identity on 2026-10-09 (`GITHUB_APP_ID` and `GITHUB_APP_INSTALLATION_ID` in `values-dev.yaml`, `GITHUB_APP_PEM` in the `claude-agent` Secret) and that credential is inert — no code path reads it. The image already carries `openssl` (so an RS256 JWT is signable) and `git` (so a clone can happen), but nothing turns the key into a token `git` will present. This helper is that step. It is deliberately the credential-helper protocol rather than a wrapper script: `git` calls a helper itself, so the token is handed to `git` over stdin/stdout and never lands in a remote URL, `.git/config`, `~/.git-credentials`, `~/.netrc` or an argv.

What is out of scope (the spec's Non-goals, restated because the executing agent has no memory of the spec): no `gh` and no general GitHub client (no PR creation, no issue API — the whole scope is "make `git` authenticate as the App"); no change to the App's permissions or installation scope; no prod wiring; no vault path (vault reads and writes stay with the `git-rest` service exactly as `agent/.claude/CLAUDE.md` § Vault specifies — this prompt gives the worker no vault credential and no vault checkout); no credential for the Claude model or the attention store; no change to `claude-headless`. In this prompt specifically: do NOT edit `Dockerfile`, do NOT edit `agent/.claude/CLAUDE.md`, do NOT edit `CHANGELOG.md`, do NOT add or edit any `.go` file, and do NOT add any dependency.

Coding guides (in-container paths, read the ones this change touches):
- `/home/node/.claude/plugins/marketplaces/coding/docs/python-cli-arguments-guide.md` — subcommand shape and the environment-variable-vs-argument split.
- `/home/node/.claude/plugins/marketplaces/coding/docs/teamvault-conventions.md` — how the Secret's value reaches the pod: `teamvaultFileBase64` yields base64-encoded file content, which is why `GITHUB_APP_PEM` is base64 and not raw PEM. Relevant to requirement 5's decode step and its recovery.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — the shared Definition of Done.
</context>

<requirements>
1. **Create the helper at `scripts/git_credential_github_app.py`, mode `0755`, starting with the shebang `#!/usr/bin/env python3`.** Python 3 standard library plus the `openssl` command only. No third-party import (`requests`, `jwt`, `cryptography`, `PyJWT`, `pem` are all forbidden — they are not in the image and the spec forbids adding a package). No Go file. No new dependency of any kind.

   The file is importable: every executable action sits behind `if __name__ == "__main__":` and the module exposes the callable surface in requirement 10 with no side effects at import time.

2. **The environment contract is exactly three variables, and nothing else in the environment may influence behaviour.**
   - `GITHUB_APP_ID` — the App id, used verbatim as the JWT's `iss`.
   - `GITHUB_APP_INSTALLATION_ID` — the installation id, used verbatim in the request path.
   - `GITHUB_APP_PEM` — base64-encoded PEM (see requirement 5).
   Declare the API base URL as a module-level constant, exactly:
   ```python
   API_BASE = "https://api.github.com"
   ```
   It must NOT be read from the environment and no environment variable may override it. A worker cannot steer the helper to a different identity or a different endpoint: the only inputs are the three variables above, and the helper takes no argument that selects an App, an installation or a host.

3. **Subcommand dispatch.** `main(argv)` takes the argument list and returns the process exit code. The first argument selects one of four subcommands:
   - `get` — requirement 4
   - `store` — a no-op that returns 0 and writes nothing. `git` calls it after a successful authentication; the helper has nothing to persist and must persist nothing.
   - `erase` — a no-op that returns 0 and writes nothing. `git` calls it when authentication fails; there is no cache to erase.
   - `token` — mints and writes the bare installation token followed by a single newline to stdout, and nothing else. Returns 0 on success.
   Any other first argument, and a missing first argument, return non-zero (use 1) and write a one-line usage diagnostic to stderr. Do NOT add a fifth subcommand, and do NOT add a differently-named alias.

4. **Nothing is cached, and nothing is shared.** Every invocation that needs a token mints a fresh one. There is no cache file, no lock file, no memoisation across invocations and no on-disk state. The token's life (one hour) is shorter than a pod's, so a fresh mint per invocation is the design; two concurrent invocations must simply each mint their own token and neither may interfere with the other. `store` and `erase` therefore have nothing to do.

5. **Decode `GITHUB_APP_PEM`, and fail loudly when you cannot.** In this order:
   (a) If `GITHUB_APP_PEM` is unset or empty, exit 1 with a stderr diagnostic that names `GITHUB_APP_PEM`. If `GITHUB_APP_ID` or `GITHUB_APP_INSTALLATION_ID` is unset or empty, exit 1 with a stderr diagnostic naming that variable. Validate all three before doing any work, and before reading stdin.
   (b) Otherwise strip ASCII whitespace from the value — the deployment produces it with the `base64` command, whose output is wrapped at 76 columns, so embedded newlines are normal input and must not be treated as corruption — then decode with `base64.b64decode(stripped, validate=True)`. If decoding raises (`binascii.Error`), exit 1 with a stderr diagnostic naming `GITHUB_APP_PEM` and the decode step. Never assume the value is raw PEM.

6. **Build and sign the JWT.** The header is `{"alg":"RS256","typ":"JWT"}`. The payload is `{"iat": now - 60, "exp": now + 540, "iss": <GITHUB_APP_ID>}`, where `now` is the current Unix time in seconds. `exp` must be at most 9 minutes (540 seconds) ahead of `now`; never use a value that reaches or exceeds 600 seconds, because GitHub rejects an `exp` more than 10 minutes ahead and the margin exists so clock skew cannot invalidate the token. Both segments are base64url-encoded with padding stripped (`base64.urlsafe_b64encode(...).rstrip(b"=")`), and the signing input is the ASCII bytes of `header_segment + "." + payload_segment`.

   Sign it with `openssl dgst -sha256 -sign <key-file>`, feeding the signing input on stdin (`subprocess.run([...], input=signing_input, capture_output=True)`). The decoded PEM must be written to a file first because `openssl -sign` needs a path: write it with `tempfile` at mode `0600` and delete it in a `finally` block so no copy of the key survives the process, on any path including failure. If `openssl` exits non-zero or is absent, exit 1 with a stderr diagnostic naming the PEM/`openssl` step — do not fall through with a truncated signature.

   Base64url-encode the signature bytes with padding stripped and join the three segments with `.`.

7. **Exchange the JWT for an installation token.** `POST` to `{API_BASE}/app/installations/{GITHUB_APP_INSTALLATION_ID}/access_tokens` with headers `Authorization: Bearer <jwt>`, `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28` and `Content-Type: application/json`, a body of `{}`, and an explicit timeout of 30 seconds. Use `urllib.request` from the standard library. Do NOT shell out to `curl`: a `curl` invocation puts the JWT in argv, and the JWT is a credential derived from a long-lived one.

   Parse the JSON response body and read its `token` field. If the field is absent or empty, treat the response as a failure (exit 5, requirement 8) rather than emitting an empty credential.

8. **Exit codes distinguish the failure classes the spec names.** Return:
   - `0` — success.
   - `1` — a local input error: a missing or empty environment variable, an undecodable `GITHUB_APP_PEM`, an `openssl` failure, or a bad/unknown subcommand.
   - `2` — the GitHub API rejected the JWT (HTTP 401). The diagnostic must make a rejected JWT distinguishable from a transport failure, because the common cause is a `GITHUB_APP_ID` that does not match the PEM's App.
   - `3` — the mint was throttled: an HTTP 403 whose response carries `X-RateLimit-Remaining: 0` or a body `message` naming a rate limit. Name the throttle; do not report it as a generic authentication failure.
   - `4` — a transport failure: `urllib.error.URLError`, a socket timeout, or any other failure to reach the host. Surface the transport error.
   - `5` — any other non-2xx API response (and the missing-`token` case from requirement 7).
   A response in the 200 range whose body has no usable `token` is exit 5, never 0.

   One class is deliberately NOT handled here: an installation that does not cover the requested repository. The mint is per-installation, not per-repo, so it succeeds and the *server* then refuses the git operation with 403/404. That refusal is the scope boundary working as designed; the helper must not attempt to classify it and must not retry.

9. **Never emit the credential, and make every diagnostic single-line and input-naming.** The helper must not print, log or otherwise emit the JWT, the decoded PEM, or the minted token on any path except the two success paths: the `password=` line of `get`, and the bare token of `token`. On success, write nothing to stderr. On failure, write one line to stderr naming the offending input or step — and the HTTP status where one applies. Never embed the raw response body of the token request in a diagnostic. There must be no `set -x`-equivalent tracing, no debug print, and no path on which the token reaches stdout while a failure is being reported.

10. **Expose a testable surface.** Keep the logic in named module-level functions so the suite in requirement 12 can drive it directly, with at least:
    - `read_env(environ)` — returns the three values, raising a locally-defined error (carrying a message that names the missing variable) when one is unset or empty.
    - `decode_pem(value)` — requirement 5 (b), returning the decoded bytes.
    - `build_jwt(app_id, pem_bytes, now)` — requirement 6, taking `now` as a parameter so the token's time bounds are assertable without patching the clock, and returning the JWT string.
    - `post_json(url, payload, headers)` — the single HTTP seam used by requirement 7.
    - `mint_installation_token(app_id, installation_id, pem_bytes)` — requirements 6 and 7 together, returning the token string or raising the error that carries the requirement 8 exit code.
    - `main(argv)` — requirement 3.
    Do not add a `--help`-style surface or any flag beyond the four subcommands.

11. **Add one make target and wire it into the existing chain.** In `Makefile.precommit`, add immediately after the `python-check` target:
    ```make
    .PHONY: python-test
    python-test:
    	@python3 -m unittest discover -s scripts -t scripts -p '*_test.py' -v
    ```
    and change the recipe line from
    ```
    precommit: ensure format generate python-check test check addlicense
    ```
    to
    ```
    precommit: ensure format generate python-check python-test test check addlicense
    ```
    Add no other target, and change nothing else in `Makefile.precommit`. (`python3` is already required by `python-check`, so this adds no new prerequisite to the repository.) Verified in-container before writing: `python3 -m unittest discover -s scripts -t scripts -p '*_test.py' -v` discovers `scripts/*_test.py`, imports the module under test, and exits 0.

12. **Write the suite at `scripts/git_credential_github_app_test.py`.** Standard-library `unittest` only (no `pytest`, no new dependency). At the top of the file, before importing the module under test, put the module's directory on `sys.path`:
    ```python
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import git_credential_github_app as helper
    ```
    so the suite passes whether it is run through the make target or directly. Drive `helper.main(argv)` and `helper.build_jwt(...)` in-process, with `helper.API_BASE` and `helper.post_json` monkeypatched, and capture stdout/stderr. Cover, at minimum:

    (a) `get` happy path — `post_json` patched to return a 200 with `{"token": "<fake>"}`; stdin is `protocol=https\nhost=github.com\n\n`; stdout contains exactly the lines `username=x-access-token` and `password=<fake>`; exit 0.
    (b) `token` happy path — stdout is `<fake>` plus a single newline and nothing else; exit 0.
    (c) `store` and `erase` — each exits 0 and writes nothing to stdout or stderr.
    (d) an unknown subcommand, and a missing subcommand — each exits non-zero, writes to stderr, and writes nothing to stdout.
    (e) each of `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PEM` unset, and each empty — four to six cases, each exiting 1, with a stderr message naming the offending variable and NO `password=` anywhere on stdout. This is the spec's Acceptance Criterion 5 and its first Failure Modes row; assert it directly.
    (f) `GITHUB_APP_PEM` set to a non-base64 string — exit 1, stderr names `GITHUB_APP_PEM`, no `password=` on stdout.
    (g) `build_jwt` against a real key, exercising the signing boundary — generate a throwaway RSA key with `openssl genrsa 2048` into a temporary directory (the suite requires `openssl` on `PATH`; do NOT skip when it is missing, fail), call `build_jwt(app_id="123", pem_bytes=<key>, now=<fixed integer>)`, and assert: the result has three dot-separated segments; each segment is valid base64url with no `=` padding; the decoded header is exactly `{"alg":"RS256","typ":"JWT"}`; the decoded payload has `iss == "123"`, `iat == now - 60` and `exp == now + 540`; and the signature round-trips through `openssl dgst -sha256 -verify <pub> -signature <sig>`, i.e. the boundary accepts what the helper produced.
    (h) base64 with embedded newlines — `GITHUB_APP_PEM` set to the same key's base64 wrapped at 76 columns (as the `base64` command emits it) mints successfully with `post_json` patched, proving the whitespace strip in requirement 5 (b) handles the deployment's real delivery shape.
    (i) HTTP 401 → exit 2; HTTP 403 carrying `X-RateLimit-Remaining: 0` → exit 3; HTTP 500 → exit 5; `post_json` raising `urllib.error.URLError` → exit 4. Each of these four writes a non-empty single-line stderr diagnostic and nothing to stdout.
    (j) no-leak assertion — on every failure case above, the attempted JWT and the PEM never appear in the captured stdout or stderr. Assert it on at least the 401 and 500 cases.

    The suite must be hermetic: no network, no real GitHub call, no reliance on `GITHUB_APP_*` being set in the ambient environment (pass an explicit environment mapping or patch the environment rather than reading the ambient one).

13. **Self-check before finishing.** Re-run every command in `<verification>` and confirm each stated result, then walk each numbered requirement above against the code you actually wrote. In particular confirm: (i) the helper imports and runs under `python3` with no third-party import; (ii) the failure paths write no `password=` line and name the offending input on stderr; (iii) `API_BASE` is a module constant and no environment variable overrides the API host; (iv) `curl` does not appear in the helper; (v) `Makefile.precommit` gained exactly one target and one recipe word; (vi) the only files you created or changed are `scripts/git_credential_github_app.py`, `scripts/git_credential_github_app_test.py` and `Makefile.precommit` — and that `Dockerfile`, `CHANGELOG.md` and `agent/.claude/CLAUDE.md` are untouched.
</requirements>

<constraints>
- **No new package, and no new Go dependency.** `openssl`, `git` and `python3` are already in the image; `agent/.claude/CLAUDE.md` forbids package installation at runtime and the image build must not add a package either. This is an image-level capability, not an application one. Do not touch `go.mod` or `go.sum`.
- **No `gh`.** Installing the GitHub CLI would widen the image's surface and is not needed to make `git` authenticate.
- **`GITHUB_APP_PEM` is base64-encoded PEM**, as delivered by the Secret (`teamvaultFileBase64`, i.e. `teamvaultFile | base64`). Decode it; never assume raw PEM.
- **The JWT lifetime must be short.** GitHub rejects an `exp` more than 10 minutes ahead; use `now + 540` seconds (9 minutes) so clock skew cannot invalidate the token. Never `>= 600`.
- **The credential must never land.** The token is handed to `git` over the credential-helper protocol on stdin/stdout. It must not reach a remote URL, `.git/config`, `~/.git-credentials`, `~/.netrc`, an argv, or a log line. `agent/.claude/CLAUDE.md` forbids a worker printing, logging or transmitting credentials, and the mechanism must not contradict the instruction the worker is given.
- **Failure is loud.** A missing or malformed `GITHUB_APP_PEM`, a missing `GITHUB_APP_ID` or `GITHUB_APP_INSTALLATION_ID`, a rejected JWT, a throttled mint or a network failure each produce a non-zero exit and a stderr diagnostic naming the offending input. Writing no `password=` line on failure is required, because a helper that returns empty lets `git` fall back to an anonymous request — which succeeds against public repos and fails against private ones, the failure mode that would make a worker believe it had pushed when it had not.
- **Wiring the helper into the image is a separate prompt.** Do NOT edit `Dockerfile`. Do NOT edit `agent/.claude/CLAUDE.md`. Do NOT edit `CHANGELOG.md` — the changelog entry for this capability is written by sibling prompt `018-spec-004-install-and-wire-credential-helper.md`. Do not add a `git config` line anywhere: the image build sets it, because `agent/.claude/CLAUDE.md` forbids a worker from modifying system config.
- **The vault is untouched.** No vault credential, no vault checkout, no vault remote. Vault reads and writes stay with the `git-rest` service.
- Change ONLY `scripts/git_credential_github_app.py`, `scripts/git_credential_github_app_test.py` and `Makefile.precommit`. No other file, and no `.go` file at all.
- Do NOT commit — dark-factory handles git. Do not run any `git` command: `.git` is masked in this container (`hideGit=true`) and every `git` invocation fails.
- Do NOT run `docker`, `kubectl`, `make build`, `make buca` or `scripts/*.sh` — this container has no Docker socket, no cluster credentials and no host tooling.
- Existing tests must still pass. No Go test is added and no Go coverage target applies, because no Go code changes; the Python suite in requirement 12 is this change's test coverage.
- Errors in any Go code you would write use `github.com/bborbe/errors` — but this prompt adds no Go code, so this rule applies only if you find yourself tempted to write some: don't.
</constraints>

<verification>
Run each command and confirm the stated result. `ROOTDIR=/workspace` pins the repository root explicitly, because `Makefile.variables` resolves `ROOTDIR` from a repository-root probe that does not work under this container's masked `.git`; the value is harmless (the `trivy` target's local `.trivyignore` branch wins) and matches every completed prompt in this repository. No `git` command appears below — `.git` is masked here.

A note on reading the results: several checks are "must be absent" and are written as `! grep -q …`, which prints nothing and exits 0 when the string is absent — that exit 0 is the expected result. `grep -c` is used only where the expected count is non-zero.

1. `ROOTDIR=/workspace make precommit` — exits 0. This is the repository's declared `validationCommand` and the exit code the completion report carries. It now also runs the new `python-test` target.
2. `python3 -m unittest discover -s scripts -t scripts -p '*_test.py' -v` — exits 0 and its output ends with `OK`. (Same command the `python-test` target runs.)
3. `test -x scripts/git_credential_github_app.py && echo executable` — prints `executable` (mode 0755).
4. `head -1 scripts/git_credential_github_app.py` — prints `#!/usr/bin/env python3`.
5. `env -u GITHUB_APP_ID -u GITHUB_APP_INSTALLATION_ID -u GITHUB_APP_PEM python3 scripts/git_credential_github_app.py get </dev/null >/tmp/req5.out 2>/tmp/req5.err; echo "rc=$?"` — prints a non-zero `rc=`; then `grep -c 'GITHUB_APP' /tmp/req5.err` prints at least `1` (a diagnostic names the offending variable) and `! grep -q 'password=' /tmp/req5.out` exits 0 (no credential line was written).
6. `GITHUB_APP_ID=1 GITHUB_APP_INSTALLATION_ID=1 GITHUB_APP_PEM='' python3 scripts/git_credential_github_app.py token >/tmp/req6.out 2>/tmp/req6.err; echo "rc=$?"` — prints a non-zero `rc=`; `grep -c 'GITHUB_APP_PEM' /tmp/req6.err` prints at least `1`; `! test -s /tmp/req6.out` exits 0 (empty stdout).
7. `printf 'protocol=https\nhost=github.com\n\n' | python3 scripts/git_credential_github_app.py store >/tmp/req7.out 2>/tmp/req7.err; echo "rc=$?"` — prints `rc=0`; `! test -s /tmp/req7.out` exits 0.
8. `printf 'protocol=https\nhost=github.com\n\n' | python3 scripts/git_credential_github_app.py erase >/tmp/req8.out 2>/tmp/req8.err; echo "rc=$?"` — prints `rc=0`; `! test -s /tmp/req8.out` exits 0.
9. `python3 scripts/git_credential_github_app.py bogus >/tmp/req9.out 2>/tmp/req9.err; echo "rc=$?"` — prints a non-zero `rc=`; `test -s /tmp/req9.err && echo has-diagnostic` prints `has-diagnostic`.
10. `GITHUB_APP_ID=1 GITHUB_APP_INSTALLATION_ID=1 GITHUB_APP_PEM='!!!not-base64!!!' python3 scripts/git_credential_github_app.py token >/tmp/req10.out 2>/tmp/req10.err; echo "rc=$?"` — prints a non-zero `rc=`; `grep -c 'GITHUB_APP_PEM' /tmp/req10.err` prints at least `1`; `! test -s /tmp/req10.out` exits 0.
11. `grep -cE '^API_BASE = "https://api\.github\.com"$' scripts/git_credential_github_app.py` — prints `1` (the API host is a module-level assignment, not a mention in a comment).
12. `! grep -qE 'environ.*GITHUB_API|getenv\(.GITHUB_API|GITHUB_API_URL' scripts/git_credential_github_app.py` — exits 0 (no environment variable overrides the API host).
13. `! grep -q 'curl' scripts/git_credential_github_app.py` — exits 0 (the mint does not shell out to `curl`; the JWT never reaches an argv).
14. `! grep -qE 'ghs_[A-Za-z0-9]|github_pat_|BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY' scripts/git_credential_github_app.py` — exits 0 (no credential value is embedded in the helper).
15. `grep -c '^python-test:' Makefile.precommit` — prints `1`; `grep -cE '^precommit: .* python-test ' Makefile.precommit` — prints `1` (the target is wired into the `precommit` recipe line itself, not merely named in a comment); `grep -A1 '^python-test:' Makefile.precommit | grep -c 'unittest discover'` — prints `1` (the target actually runs the suite, so an empty or stubbed recipe cannot pass).
16. `test -f scripts/git_credential_github_app_test.py && echo present` — prints `present` (the suite exists where the make target looks for it).
17. `! grep -q 'curl' scripts/git_credential_github_app_test.py` — exits 0.
</verification>

<success_criteria>
- `scripts/git_credential_github_app.py` exists, is mode 0755, starts with `#!/usr/bin/env python3`, and imports nothing outside the Python 3 standard library.
- It serves `get` (git credential-helper protocol, writing `username=x-access-token` and `password=<token>` on success), `store` and `erase` (no-ops returning 0), and `token` (bare token plus newline). No fifth subcommand and no alias.
- It reads exactly `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID` and `GITHUB_APP_PEM`; the API host is a module constant `API_BASE = "https://api.github.com"` and no environment variable overrides it.
- `GITHUB_APP_PEM` is whitespace-stripped and base64-decoded with `validate=True`; a missing, empty or undecodable value exits non-zero with a stderr diagnostic naming the offending input and writes no `password=` line.
- The JWT uses `alg=RS256`, `iss=<GITHUB_APP_ID>`, `iat=now-60` and `exp=now+540`, is signed through `openssl dgst -sha256 -sign` with the decoded key held in a mode-0600 temp file that is deleted in a `finally` block, and the mint POSTs to `{API_BASE}/app/installations/{id}/access_tokens` through `urllib.request` with a 30-second timeout — never through `curl`.
- Exit codes distinguish: 1 local input error, 2 JWT rejected (401), 3 throttled (403 + rate limit), 4 transport failure, 5 any other non-2xx or a 2xx body with no usable `token`.
- Nothing is cached between invocations; there is no cache file, lock or shared state.
- The token, the JWT and the decoded PEM appear only on the two success paths; every failure diagnostic is one line on stderr naming the input or step, and never embeds the token request's response body.
- `scripts/git_credential_github_app_test.py` runs under stdlib `unittest`, is hermetic (no network, no ambient `GITHUB_APP_*` dependence), and covers the protocol, the three subcommand no-ops, every missing/empty variable, the undecodable key, a real `openssl` signing round-trip, a 76-column-wrapped key, the 401/403-throttle/500/transport exit codes, and a no-leak assertion.
- `Makefile.precommit` gains exactly one `.PHONY: python-test` target and the word `python-test` in its `precommit` recipe, and nothing else changes in it.
- Only `scripts/git_credential_github_app.py`, `scripts/git_credential_github_app_test.py` and `Makefile.precommit` are created or modified; `Dockerfile`, `CHANGELOG.md`, `agent/.claude/CLAUDE.md`, every `.go` file, `go.mod` and `go.sum` are untouched.
- `ROOTDIR=/workspace make precommit` exits 0, and every command in `<verification>` prints the stated result.
</success_criteria>
