#!/bin/bash
# vault-complete_test.sh — fixture-driven test of set_status_completed, no server.
# Sources vault-complete.sh and exercises the function against temp fixtures,
# then runs the helper as a process against a stub curl.
# Usage: vault-complete_test.sh

set -u -o pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${here}/vault-complete.sh"

set +e +u +o pipefail

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0
pass() { echo "PASS: $1"; }
bad() { echo "FAIL: $1"; fail=1; }

# --- replace: only the frontmatter status line changes ----------------------
cat > "${tmp}/file_replace.md" <<'EOF'
---
title: Do the thing
status: in_progress
tags:
  - a
---

# Progress

status: draft
some body line
EOF

set_status_completed "${tmp}/file_replace.md" > "${tmp}/out_replace.md"
rc=$?

if [ "$rc" -eq 0 ] \
  && grep -q '^status: completed$' "${tmp}/out_replace.md" \
  && grep -q '^title: Do the thing$' "${tmp}/out_replace.md" \
  && grep -q '^status: draft$' "${tmp}/out_replace.md" \
  && [ "$(grep -c '^status:' "${tmp}/out_replace.md")" -eq 2 ] \
  && [ "$(tail -c 1 "${tmp}/out_replace.md" | wc -l)" -eq 1 ]; then
  pass "replace: frontmatter status set, other keys and body status line intact, trailing newline kept"
else
  bad "replace: frontmatter status set, other keys and body status line intact, trailing newline kept"
fi

# --- idempotent: an already-completed file comes through byte-identical ------
printf -- '---\ntitle: Done\nstatus: completed\n---\n\n# Progress\n\nbody without trailing newline' > "${tmp}/file_done.md"

set_status_completed "${tmp}/file_done.md" > "${tmp}/out_done.md"
rc=$?

if [ "$rc" -eq 0 ] && cmp -s "${tmp}/file_done.md" "${tmp}/out_done.md"; then
  pass "idempotent: already completed yields identical bytes, trailing newline state preserved"
else
  bad "idempotent: already completed yields identical bytes, trailing newline state preserved"
fi

# --- no frontmatter: fail loudly, name the file -----------------------------
printf '# Title\n\nbody\n' > "${tmp}/file_nofm.md"

set_status_completed "${tmp}/file_nofm.md" > "${tmp}/out_nofm.md" 2> "${tmp}/err_nofm.txt"
rc=$?

if [ "$rc" -ne 0 ] && grep -q 'file_nofm.md' "${tmp}/err_nofm.txt"; then
  pass "no frontmatter: non-zero exit and file named on stderr"
else
  bad "no frontmatter: non-zero exit and file named on stderr (rc=${rc})"
fi

# --- no status key: fail loudly ---------------------------------------------
printf -- '---\ntitle: x\n---\n\nbody\n' > "${tmp}/file_nostatus.md"

set_status_completed "${tmp}/file_nostatus.md" > "${tmp}/out_nostatus.md" 2> "${tmp}/err_nostatus.txt"
rc=$?

if [ "$rc" -ne 0 ] && grep -q 'file_nostatus.md' "${tmp}/err_nostatus.txt"; then
  pass "no status key: non-zero exit and file named on stderr"
else
  bad "no status key: non-zero exit and file named on stderr (rc=${rc})"
fi

# --- duplicate status: fail loudly ------------------------------------------
printf -- '---\nstatus: next\nstatus: hold\n---\n\nbody\n' > "${tmp}/file_dup.md"

set_status_completed "${tmp}/file_dup.md" > "${tmp}/out_dup.md" 2> "${tmp}/err_dup.txt"
rc=$?

if [ "$rc" -ne 0 ] && grep -q 'file_dup.md' "${tmp}/err_dup.txt"; then
  pass "duplicate status: non-zero exit and file named on stderr"
else
  bad "duplicate status: non-zero exit and file named on stderr (rc=${rc})"
fi

# --- unclosed frontmatter: fail loudly --------------------------------------
printf -- '---\nstatus: next\n\nbody\n' > "${tmp}/file_unclosed.md"

set_status_completed "${tmp}/file_unclosed.md" > "${tmp}/out_unclosed.md" 2> "${tmp}/err_unclosed.txt"
rc=$?

if [ "$rc" -ne 0 ] && grep -q 'file_unclosed.md' "${tmp}/err_unclosed.txt"; then
  pass "unclosed frontmatter: non-zero exit and file named on stderr"
else
  bad "unclosed frontmatter: non-zero exit and file named on stderr (rc=${rc})"
fi

# --- process: GET, modify, POST through a stub curl -------------------------
mkdir -p "${tmp}/bin" "${tmp}/spill"

printf -- '---\ntitle: Proc\nstatus: in_progress\n---\n\n# Progress\n\nstatus: draft\n' > "${tmp}/remote.md"

cat > "${tmp}/bin/curl" <<'STUB'
#!/bin/bash
# Stub curl — never touches the network. Records argv, captures the POST body
# file, and either cats the GET fixture or prints the gateway's ok body.
printf '%s\n' "$*" >> "${VAULT_TEST_ARGV:?}"
is_post=0
prev=""
for a in "$@"; do
  if [ "$prev" = "-X" ] && [ "$a" = "POST" ]; then is_post=1; fi
  prev="$a"
done
if [ "$is_post" = "1" ]; then
  if [ -e "${VAULT_TEST_FAIL_MARKER:?}" ]; then
    exit 22
  fi
  prev=""
  for a in "$@"; do
    if [ "$prev" = "--data-binary" ]; then
      cp "${a#@}" "${VAULT_TEST_CAPTURE:?}"
    fi
    prev="$a"
  done
  printf '{"ok":true}'
else
  cat "${VAULT_TEST_REMOTE:?}"
fi
STUB
chmod +x "${tmp}/bin/curl"

export VAULT_TEST_REMOTE="${tmp}/remote.md"
export VAULT_TEST_FAIL_MARKER="${tmp}/fail-post"
export VAULT_TEST_ARGV="${tmp}/argv.txt"
export VAULT_TEST_CAPTURE="${tmp}/captured.md"

# TMPDIR points at a private directory so a leak is actually measured.
spill_count() {
  local n=0 f
  for f in "${tmp}"/spill/tmp.*; do
    [ -e "$f" ] && n=$((n + 1))
  done
  printf '%s' "$n"
}

before="$(spill_count)"
PATH="${tmp}/bin:${PATH}" GATEWAY_SECRET=dummy TMPDIR="${tmp}/spill" \
  bash "${here}/vault-complete.sh" 'tasks/proc.md' \
  > "${tmp}/proc_out.txt" 2> "${tmp}/proc_err.txt"
rc=$?
after="$(spill_count)"

if [ "$rc" -eq 0 ] \
  && grep -q '{"ok":true}' "${tmp}/proc_out.txt" \
  && [ ! -s "${tmp}/proc_err.txt" ]; then
  pass "process: successful write exits 0, prints the ok body, writes no stderr"
else
  bad "process: successful write exits 0, prints the ok body, writes no stderr (rc=${rc}, stderr=$(cat "${tmp}/proc_err.txt"))"
fi

if grep -q 'tasks/proc.md' "${tmp}/argv.txt" \
  && grep -q 'X-Gateway-Initator: agent-claude' "${tmp}/argv.txt" \
  && grep -q 'X-Gateway-Secret' "${tmp}/argv.txt"; then
  pass "process: GET and POST carry the path, the initiator header and the secret header"
else
  bad "process: GET and POST carry the path, the initiator header and the secret header"
fi

if grep -q -- '-X POST' "${tmp}/argv.txt" \
  && grep -q -- '--data-binary' "${tmp}/argv.txt" \
  && grep -q '^status: completed$' "${tmp}/captured.md" \
  && grep -q '^status: draft$' "${tmp}/captured.md"; then
  pass "process: POST uses --data-binary and sends the completed frontmatter with the body untouched"
else
  bad "process: POST uses --data-binary and sends the completed frontmatter with the body untouched"
fi

if [ "$after" -eq "$before" ]; then
  pass "process: no temporary files left behind on the success path"
else
  bad "process: no temporary files left behind on the success path (before=${before} after=${after})"
fi

# Missing secret: fails before any request, with the :? message.
env -u GATEWAY_SECRET PATH="${tmp}/bin:${PATH}" TMPDIR="${tmp}/spill" \
  bash "${here}/vault-complete.sh" 'tasks/proc.md' \
  > "${tmp}/nosecret_out.txt" 2> "${tmp}/nosecret_err.txt"
rc_nosecret=$?

if [ "$rc_nosecret" -ne 0 ] && grep -q 'GATEWAY_SECRET is required' "${tmp}/nosecret_err.txt"; then
  pass "process: missing GATEWAY_SECRET exits non-zero with the required message"
else
  bad "process: missing GATEWAY_SECRET exits non-zero with the required message (rc=${rc_nosecret})"
fi

: > "${tmp}/fail-post"
before_fail="$(spill_count)"
PATH="${tmp}/bin:${PATH}" GATEWAY_SECRET=dummy TMPDIR="${tmp}/spill" \
  bash "${here}/vault-complete.sh" 'tasks/proc.md' \
  > "${tmp}/fail_out.txt" 2> "${tmp}/fail_err.txt"
rc_fail=$?
after_fail="$(spill_count)"

if [ "$rc_fail" -ne 0 ] && [ "$after_fail" -eq "$before_fail" ]; then
  pass "process: failing POST exits non-zero and leaves no temporary files behind"
else
  bad "process: failing POST exits non-zero and leaves no temporary files behind (rc=${rc_fail} before=${before_fail} after=${after_fail})"
fi

exit "$fail"
