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

exit "$fail"
