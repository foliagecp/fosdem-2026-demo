#!/usr/bin/env bash
set -euo pipefail

# Emulates: `vagrant ssh <VM_ID> && sudo -i && lshw -json` executed inside a VM.
# The VM identity is passed by the agent as the first argument.

VM_ID="${1:-}"
if [[ -z "$VM_ID" ]]; then
  echo '{"error":"missing vm id"}'
  exit 0
fi

FILE="/opt/data/virtual_machine/lshw/${VM_ID}.json"
if [[ ! -f "$FILE" ]]; then
  echo '{"error":"unknown vm"}'
  exit 0
fi

cat "$FILE"
