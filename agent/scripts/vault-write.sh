#!/bin/bash
# vault-write.sh — POST a whole local file to the vault, replacing the remote file.
# Usage: vault-write.sh <repo-relative-path> <local-file>
#
# VAULT_API_URL selects the vault service (default: the nuke-era
# `vault-obsidian-personal`). GATEWAY_SECRET is OPTIONAL: the header is sent only
# when the variable is set, because git-rest leaves its API unauthenticated when
# it is configured with no secret, and requiring one here made the helper
# unusable on a cluster whose service has none.

set -e -u -o pipefail

: "${VAULT_API_URL:=http://vault-obsidian-personal:9090}"

if [ "$#" -ne 2 ]; then
  echo "usage: vault-write.sh <repo-relative-path> <local-file>" >&2
  exit 2
fi

path="${1// /%20}"

headers=(-H "X-Gateway-Initator: agent-claude")
if [ -n "${GATEWAY_SECRET:-}" ]; then
  headers+=(-H "X-Gateway-Secret: $GATEWAY_SECRET")
fi

exec curl -sf --max-time 30 \
  -X POST \
  "${headers[@]}" \
  -H 'Content-Type: application/octet-stream' \
  --data-binary "@${2}" \
  "${VAULT_API_URL}/api/v1/files/${path}"
