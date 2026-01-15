#!/usr/bin/env bash
set -euo pipefail

# Demo remediation script: "run a VM on demand"
# - Adds a VM entry to local emulated `vagrant global-status --machine-readable`
# - Creates an lshw snapshot for this VM so the agent can publish VM attributes
#
# This VM will "appear" only on the server where you execute this script.

# --- NEW: time-based dynamic VM_ID/VM_HOME ---
NOW_EPOCH="$(date +%s)"
NOW_NS="$(date +%s%N 2>/dev/null || true)"
if [[ -z "${NOW_NS}" || "${NOW_NS}" == *N* ]]; then
  NOW_NS="${NOW_EPOCH}000000000"
fi

HOST="${HOSTNAME:-$(hostname 2>/dev/null || echo unknown)}"
SEED="${NOW_NS}-${HOST}-$$-${RANDOM}-${RANDOM}"

# Prefer sha1sum; fallback to shasum (mac), then md5sum
if command -v sha1sum >/dev/null 2>&1; then
  VM_ID="$(printf '%s' "$SEED" | sha1sum | awk '{print substr($1,1,7)}')"
elif command -v shasum >/dev/null 2>&1; then
  VM_ID="$(printf '%s' "$SEED" | shasum -a 1 | awk '{print substr($1,1,7)}')"
elif command -v md5sum >/dev/null 2>&1; then
  VM_ID="$(printf '%s' "$SEED" | md5sum | awk '{print substr($1,1,7)}')"
else
  # last resort: still time-based, but not hex-pretty
  VM_ID="${NOW_EPOCH}$(printf '%x' "$$")"
fi

VM_HOME="/root/vagrant-kvm/vagrant-run-vm-${VM_ID}"
VM_STATE="running"

VAGRANT_FILE="/opt/data/virtual_machine/vagrant_global-status.txt"
LSHW_DIR="/opt/data/virtual_machine/lshw"
LSHW_FILE="${LSHW_DIR}/${VM_ID}.json"

mkdir -p "$(dirname "$VAGRANT_FILE")" "$LSHW_DIR"

# If already present, do nothing.
if [[ -f "$VAGRANT_FILE" ]] && grep -q ",,machine-id,${VM_ID}$" "$VAGRANT_FILE"; then
  echo "OK (already running): VM_ID=${VM_ID}"
  exit 0
fi

# Pick a timestamp that matches the file for nicer diffs.
ts=""
if [[ -f "$VAGRANT_FILE" ]]; then
  ts="$(head -n 1 "$VAGRANT_FILE" | cut -d',' -f1 | tr -d '[:space:]')"
fi
if [[ -z "$ts" ]]; then
  ts="$(date +%s)"
fi

# Ensure file exists with minimal metadata.
if [[ ! -f "$VAGRANT_FILE" ]]; then
  printf '%s,,metadata,machine-count,0
' "$ts" > "$VAGRANT_FILE"
fi

# Update machine-count metadata (+1).
tmp="$(mktemp)"
awk -F',' -v OFS=',' '
  $3=="metadata" && $4=="machine-count" { $5 = ($5+1) }
  { print }
' "$VAGRANT_FILE" > "$tmp"
mv "$tmp" "$VAGRANT_FILE"

# Append a new VM block.
cat >> "$VAGRANT_FILE" <<EOF

${ts},,machine-id,${VM_ID}
${ts},,provider-name,libvirt
${ts},,machine-home,${VM_HOME}
${ts},,state,${VM_STATE}
EOF

# Create lshw snapshot for the new VM ID.
# We reuse an existing VM lshw snapshot as a template and override top-level id.
if [[ ! -f "$LSHW_FILE" ]]; then
  tpl="$(ls -1 "${LSHW_DIR}"/*.json 2>/dev/null | head -n 1 || true)"
  if [[ -n "$tpl" ]]; then
    jq --arg nid "$(basename "$VM_HOME")" '.id = $nid' "$tpl" > "$LSHW_FILE" || true
  fi
fi

# Fallback if template copy failed or directory was empty.
if [[ ! -f "$LSHW_FILE" ]]; then
  cat > "$LSHW_FILE" <<EOF
{
  "id": "$(basename "$VM_HOME")",
  "class": "system",
  "description": "Computer",
  "vendor": "QEMU",
  "product": "Standard PC",
  "children": [
    {"id":"cpu:0","class":"processor","description":"CPU","vendor":"GenuineIntel","capabilities":{"vmx":"VT-x"}},
    {"id":"firmware","class":"memory","description":"BIOS","vendor":"SeaBIOS","version":"1.15.0-1","date":"04/01/2014"}
  ]
}
EOF
fi

echo "OK: VM added to vagrant snapshot (VM_ID=${VM_ID})"
