#!/usr/bin/env bash
set -euo pipefail

# Emulates: lshw -json executed inside a VM.
# The VM identity is passed by the agent as the first argument.

VM_NAME="${1:-}"
if [[ -z "$VM_NAME" ]]; then
  echo '{"error":"missing vm name"}'
  exit 0
fi

FILE="/opt/data/virtual_machine/lshw/${VM_NAME}.json"
if [[ ! -f "$FILE" ]]; then
  echo '{"error":"unknown vm"}'
  exit 0
fi

cat "$FILE"
