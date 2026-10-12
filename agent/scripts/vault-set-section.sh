#!/bin/bash
# vault-set-section.sh — GET a vault file, replace or append exactly one named
# section, and POST the whole result back. Every other byte is left untouched.
# Usage: vault-set-section.sh <repo-relative-path> <heading> <content-file>

set -e -u -o pipefail

# splice_section <file> <heading> <content-file> — emit the spliced file to stdout.
#
# The heading is taken verbatim: a bare name is not promoted to a "## " heading,
# because vault task and goal files use H1 sections and a mismatch would append a
# duplicate instead of replacing the section that already exists.
#
# Exactly one line whose content, with trailing whitespace stripped, equals the
# heading is a match. Zero matches append the heading and content at the end;
# one match replaces the heading line and every line after it up to (but not
# including) the next line whose heading level is the same or higher; two or
# more matches fail loudly rather than guessing.
splice_section() {
  local file="$1"
  local heading="$2"
  local content_file="$3"
  local count

  count=$(awk -v h="$heading" '
    { line = $0; sub(/[ \t]+$/, "", line); if (line == h) n++ }
    END { print n + 0 }
  ' "$file")

  if [ "$count" -ge 2 ]; then
    printf 'vault-set-section: heading "%s" appears %d times in %s — refusing to guess which one\n' \
      "$heading" "$count" "$file" >&2
    return 1
  fi

  if [ "$count" -eq 0 ]; then
    cat "$file"
    if [ -s "$file" ] && [ -n "$(tail -c 1 "$file")" ]; then
      printf '\n'
    fi
    printf '%s\n' "$heading"
    cat "$content_file"
    return 0
  fi

  awk -v h="$heading" -v cf="$content_file" '
    BEGIN {
      lvl = 0
      while (substr(h, lvl + 1, 1) == "#") lvl++
    }
    {
      line = $0
      sub(/[ \t]+$/, "", line)
      if (!done && line == h) {
        print h
        while ((getline cline < cf) > 0) print cline
        close(cf)
        skip = 1
        done = 1
        next
      }
      if (skip) {
        n = 0
        while (substr($0, n + 1, 1) == "#") n++
        if (n > 0 && substr($0, n + 1, 1) == " " && n <= lvl) {
          skip = 0
          print
        }
        next
      }
      print
    }
  ' "$file"
}

main() {
  : "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"

  if [ "$#" -ne 3 ]; then
    echo "usage: vault-set-section.sh <repo-relative-path> <heading> <content-file>" >&2
    exit 2
  fi

  local path="${1// /%20}"
  local heading="$2"
  local content_file="$3"
  local initiator_header="X-Gateway-Initator: agent-claude"
  local secret_header="X-Gateway-Secret: $GATEWAY_SECRET"
  local base="http://vault-obsidian-personal:9090"
  local tmp_src tmp_out

  tmp_src=$(mktemp)
  tmp_out=$(mktemp)
  trap 'rm -f "$tmp_src" "$tmp_out"' EXIT

  curl -sf --max-time 30 \
    -H "$initiator_header" \
    -H "$secret_header" \
    "${base}/api/v1/files/${path}" > "$tmp_src"

  splice_section "$tmp_src" "$heading" "$content_file" > "$tmp_out"

  curl -sf --max-time 30 \
    -X POST \
    -H "$initiator_header" \
    -H "$secret_header" \
    -H 'Content-Type: application/octet-stream' \
    --data-binary "@${tmp_out}" \
    "${base}/api/v1/files/${path}"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
