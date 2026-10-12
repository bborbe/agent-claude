#!/bin/bash
# vault-write.sh — POST a whole local file to the vault, replacing the remote file.
# Usage: vault-write.sh <repo-relative-path> <local-file>

set -e -u -o pipefail

: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"

if [ "$#" -ne 2 ]; then
  echo "usage: vault-write.sh <repo-relative-path> <local-file>" >&2
  exit 2
fi

path="${1// /%20}"

exec curl -sf --max-time 30 \
  -X POST \
  -H "X-Gateway-Initator: agent-claude" \
  -H "X-Gateway-Secret: $GATEWAY_SECRET" \
  -H 'Content-Type: application/octet-stream' \
  --data-binary "@${2}" \
  "http://vault-obsidian-personal:9090/api/v1/files/${path}"
