#!/bin/bash
# vault-complete.sh — GET a vault task file, set the task's status to completed
# in its YAML frontmatter, and POST the whole file back. Every other byte is
# left untouched, including body lines that happen to begin with "status:".
# Usage: vault-complete.sh <repo-relative-path>

set -e -u -o pipefail

# The two temporary paths live at script scope, not inside main, because an EXIT
# trap fires when the *script* exits — after main has returned and its locals are
# gone. Under `set -u` a trap reading a vanished `local` raises `unbound variable`
# and turns a successful write into exit 1; it also skips the `rm -f`, leaking
# both files. Hoisted here, the trap resolves them on every exit path.
tmp_src=""
tmp_out=""

cleanup() {
  rm -f "${tmp_src:-}" "${tmp_out:-}"
}

trap cleanup EXIT

# set_status_completed <file> — emit the file with its frontmatter status set to
# completed on stdout. The frontmatter is the region from line 1 (which must be
# exactly "---") through the next line that is exactly "---". Exactly one line
# in that region may match "^status:"; zero or two or more fail loudly, because
# guessing would rewrite the wrong line or silently drop a key. The final
# newline state of the input is preserved.
set_status_completed() {
  local file="$1"
  local final_nl=1

  if [ -s "$file" ] && [ -n "$(tail -c 1 "$file")" ]; then
    final_nl=0
  fi

  awk -v final_nl="$final_nl" -v file="$file" '
    { lines[NR] = $0 }
    END {
      if (NR == 0 || lines[1] != "---") {
        printf "vault-complete: %s has no YAML frontmatter: line 1 is not ---\n", file > "/dev/stderr"
        exit 1
      }
      close_at = 0
      for (i = 2; i <= NR; i++) {
        if (lines[i] == "---") { close_at = i; break }
      }
      if (close_at == 0) {
        printf "vault-complete: %s has an unclosed YAML frontmatter: no closing ---\n", file > "/dev/stderr"
        exit 1
      }
      count = 0
      target = 0
      for (i = 2; i < close_at; i++) {
        if (lines[i] ~ /^status:/) { count++; target = i }
      }
      if (count == 0) {
        printf "vault-complete: %s has no status key in its frontmatter\n", file > "/dev/stderr"
        exit 1
      }
      if (count > 1) {
        printf "vault-complete: %s has %d status keys in its frontmatter — refusing to guess\n", file, count > "/dev/stderr"
        exit 1
      }
      for (i = 1; i <= NR; i++) {
        if (i == target) lines[i] = "status: completed"
        if (i == NR && final_nl == 0) printf "%s", lines[i]
        else print lines[i]
      }
    }
  ' "$file"
}

main() {
  : "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"

  if [ "$#" -ne 1 ]; then
    echo "usage: vault-complete.sh <repo-relative-path>" >&2
    exit 2
  fi

  local path="${1// /%20}"
  local initiator_header="X-Gateway-Initator: agent-claude"
  local secret_header="X-Gateway-Secret: $GATEWAY_SECRET"
  local base="http://vault-obsidian-personal:9090"

  tmp_src=$(mktemp)
  tmp_out=$(mktemp)

  curl -sf --max-time 30 \
    -H "$initiator_header" \
    -H "$secret_header" \
    "${base}/api/v1/files/${path}" > "$tmp_src"

  set_status_completed "$tmp_src" > "$tmp_out"

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
