#!/bin/bash
# vault-set-section_test.sh — fixture-driven test of splice_section, no server.
# Sources vault-set-section.sh and exercises the function against temp fixtures.
# Usage: vault-set-section_test.sh

set -u -o pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "${here}/vault-set-section.sh"

set +e +u +o pipefail

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0
pass() { echo "PASS: $1"; }
bad() { echo "FAIL: $1"; fail=1; }

# --- replace: vault-shaped H1 sections -------------------------------------
cat > "${tmp}/file_replace.md" <<'EOF'
# Tasks

- [ ] one
- [ ] two

# Progress

old progress line 1
old progress line 2

# Related

[[Some Page]]
EOF

printf 'new progress line\n' > "${tmp}/content_replace.md"

splice_section "${tmp}/file_replace.md" '# Progress' "${tmp}/content_replace.md" > "${tmp}/out_replace.md"

if grep -q '^# Tasks$' "${tmp}/out_replace.md" \
  && grep -q '^- \[ \] one$' "${tmp}/out_replace.md" \
  && grep -q '^# Related$' "${tmp}/out_replace.md" \
  && grep -q '^\[\[Some Page\]\]$' "${tmp}/out_replace.md" \
  && grep -q '^# Progress$' "${tmp}/out_replace.md" \
  && grep -q '^new progress line$' "${tmp}/out_replace.md" \
  && ! grep -q 'old progress line' "${tmp}/out_replace.md"; then
  pass "replace: H1 section body replaced, other sections intact"
else
  bad "replace: H1 section body replaced, other sections intact"
fi

# --- replace: sub-headings inside the section are wholly replaced -----------
cat > "${tmp}/file_impact.md" <<'EOF'
# Impact

## Sub

sub body

# Other

other body
EOF

printf 'replaced impact\n' > "${tmp}/content_impact.md"

splice_section "${tmp}/file_impact.md" '# Impact' "${tmp}/content_impact.md" > "${tmp}/out_impact.md"

if grep -q '^# Impact$' "${tmp}/out_impact.md" \
  && grep -q '^replaced impact$' "${tmp}/out_impact.md" \
  && grep -q '^# Other$' "${tmp}/out_impact.md" \
  && ! grep -q '^## Sub$' "${tmp}/out_impact.md" \
  && ! grep -q '^sub body$' "${tmp}/out_impact.md"; then
  pass "replace: same-level boundary spans sub-headings"
else
  bad "replace: same-level boundary spans sub-headings"
fi

# --- append: no matching heading, file has no trailing newline --------------
printf '# Notes\n\nsome text without newline' > "${tmp}/file_append.md"
printf 'appended body\n' > "${tmp}/content_append.md"

splice_section "${tmp}/file_append.md" '# Added' "${tmp}/content_append.md" > "${tmp}/out_append.md"

if grep -q '^# Notes$' "${tmp}/out_append.md" \
  && grep -q '^some text without newline$' "${tmp}/out_append.md" \
  && grep -q '^# Added$' "${tmp}/out_append.md" \
  && grep -q '^appended body$' "${tmp}/out_append.md" \
  && ! grep -q 'some text without newline# Added' "${tmp}/out_append.md"; then
  pass "append: new section added, prior last line not fused"
else
  bad "append: new section added, prior last line not fused"
fi

# --- duplicate heading: fail loudly ----------------------------------------
cat > "${tmp}/file_dup.md" <<'EOF'
# Progress

a

# Progress

b
EOF

printf 'x\n' > "${tmp}/content_dup.md"

splice_section "${tmp}/file_dup.md" '# Progress' "${tmp}/content_dup.md" > "${tmp}/out_dup.md" 2> "${tmp}/err_dup.txt"
rc=$?

if [ "$rc" -ne 0 ] && grep -q 'Progress' "${tmp}/err_dup.txt"; then
  pass "duplicate: non-zero exit and heading named on stderr"
else
  bad "duplicate: non-zero exit and heading named on stderr"
fi

# --- idempotency: splicing the same content twice is stable -----------------
splice_section "${tmp}/file_replace.md" '# Progress' "${tmp}/content_replace.md" > "${tmp}/out_once.md"
splice_section "${tmp}/out_once.md" '# Progress' "${tmp}/content_replace.md" > "${tmp}/out_twice.md"

if diff -q "${tmp}/out_once.md" "${tmp}/out_twice.md" > /dev/null; then
  pass "idempotent: second splice yields the same file"
else
  bad "idempotent: second splice yields the same file"
fi

# --- helper as a process: exit code, and cleanup on both exit paths ---------
#
# Every case above calls splice_section directly, so none of them reaches main's
# exit path — and that is exactly where the defect lives. The EXIT trap fired
# when the *script* exited, after main had returned and its `local` temp paths
# had gone out of scope; under `set -u` the trap's `"$tmp_src"` raised `unbound
# variable`, so a write that had already succeeded (the POST's `{"ok":true}` was
# on stdout) still exited 1, and because the trap died before its `rm -f` both
# mktemp files leaked. A sourcing test cannot reproduce it: the `set +e +u` above
# disarms the trap's expansion, so the helper has to be run as its own process.
mkdir -p "${tmp}/bin" "${tmp}/spill"

printf '# Notes\n\nold body\n' > "${tmp}/remote.md"
printf 'new body\n' > "${tmp}/content_proc.md"

cat > "${tmp}/bin/curl" <<'STUB'
#!/bin/bash
# Stub curl — never touches the network. A POST prints the gateway's ok body
# unless the failure marker exists; a GET cats the fixture, so the helper's
# `curl … > "$tmp_src"` captures a real file to splice.
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
  printf '{"ok":true}'
else
  cat "${VAULT_TEST_REMOTE:?}"
fi
STUB
chmod +x "${tmp}/bin/curl"

export VAULT_TEST_REMOTE="${tmp}/remote.md"
export VAULT_TEST_FAIL_MARKER="${tmp}/fail-post"

# TMPDIR points at a private directory so the leak is actually measured: a bare
# ${TMPDIR} is empty in a container that does not set it, which would make the
# count 0 before and after and pass without observing anything.
spill_count() {
  local n=0 f
  for f in "${tmp}"/spill/tmp.*; do
    [ -e "$f" ] && n=$((n + 1))
  done
  printf '%s' "$n"
}

before="$(spill_count)"
PATH="${tmp}/bin:${PATH}" GATEWAY_SECRET=dummy TMPDIR="${tmp}/spill" \
  bash "${here}/vault-set-section.sh" 'notes/proc.md' '# Notes' "${tmp}/content_proc.md" \
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

if [ "$after" -eq "$before" ]; then
  pass "process: no temporary files left behind on the success path"
else
  bad "process: no temporary files left behind on the success path (before=${before} after=${after})"
fi

# Missing secret: fails before any request, with the explicit guard. The exit
# status alone is not evidence here — bash 5 already exits 1 on the old `:?`
# form — so the case also asserts the guard is no longer that form.
env -u GATEWAY_SECRET PATH="${tmp}/bin:${PATH}" TMPDIR="${tmp}/spill" \
  bash "${here}/vault-set-section.sh" 'notes/proc.md' '# Notes' "${tmp}/content_proc.md" \
  > "${tmp}/nosecret_out.txt" 2> "${tmp}/nosecret_err.txt"
rc_nosecret=$?

if [ "$rc_nosecret" -ne 0 ] \
  && grep -q 'GATEWAY_SECRET is required' "${tmp}/nosecret_err.txt" \
  && ! grep -qF ': "${GATEWAY_SECRET:?' "${here}/vault-set-section.sh"; then
  pass "process: missing GATEWAY_SECRET exits non-zero with the required message"
else
  bad "process: missing GATEWAY_SECRET exits non-zero with the required message (rc=${rc_nosecret})"
fi

# The shipped helper already cleans up on this path, because `set -e` exits from
# inside main with its locals still live. Kept as a guard on the fix: a cleanup
# moved to the end of main would satisfy the two checks above and leak here.
: > "${tmp}/fail-post"
before_fail="$(spill_count)"
PATH="${tmp}/bin:${PATH}" GATEWAY_SECRET=dummy TMPDIR="${tmp}/spill" \
  bash "${here}/vault-set-section.sh" 'notes/proc.md' '# Notes' "${tmp}/content_proc.md" \
  > "${tmp}/fail_out.txt" 2> "${tmp}/fail_err.txt"
rc_fail=$?
after_fail="$(spill_count)"

if [ "$rc_fail" -ne 0 ] && [ "$after_fail" -eq "$before_fail" ]; then
  pass "process: no temporary files left behind on the failure path"
else
  bad "process: no temporary files left behind on the failure path (rc=${rc_fail} before=${before_fail} after=${after_fail})"
fi

exit "$fail"
