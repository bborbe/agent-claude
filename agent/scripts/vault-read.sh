#!/bin/bash
# vault-read.sh — GET one file from the vault and write its raw bytes to stdout.
# Usage: vault-read.sh <repo-relative-path>

set -e -u -o pipefail

: "${GATEWAY_SECRET:?GATEWAY_SECRET is required}"

if [ "$#" -ne 1 ]; then
  echo "usage: vault-read.sh <repo-relative-path>" >&2
  exit 2
fi

path="${1// /%20}"

exec curl -sf --max-time 30 \
  -H "X-Gateway-Initator: agent-claude" \
  -H "X-Gateway-Secret: $GATEWAY_SECRET" \
  "http://vault-obsidian-personal:9090/api/v1/files/${path}"
