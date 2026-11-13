#!/bin/bash
# vault-read.sh — GET one file from the vault and write its raw bytes to stdout.
# Usage: vault-read.sh <repo-relative-path>
#
# VAULT_API_URL selects the vault service (default: the nuke-era
# `vault-obsidian-personal`). A cluster that runs its own git-rest — sun does —
# points this at that Service instead. GATEWAY_SECRET is OPTIONAL: the header is
# sent only when the variable is set, because git-rest leaves its API
# unauthenticated when it is configured with no secret, and requiring one here
# made the helper unusable on a cluster whose service has none.

set -e -u -o pipefail

: "${VAULT_API_URL:=http://vault-obsidian-personal:9090}"

if [ "$#" -ne 1 ]; then
  echo "usage: vault-read.sh <repo-relative-path>" >&2
  exit 2
fi

path="${1// /%20}"

headers=(-H "X-Gateway-Initator: agent-claude")
if [ -n "${GATEWAY_SECRET:-}" ]; then
  headers+=(-H "X-Gateway-Secret: $GATEWAY_SECRET")
fi

exec curl -sf --max-time 30 \
  "${headers[@]}" \
  "${VAULT_API_URL}/api/v1/files/${path}"
