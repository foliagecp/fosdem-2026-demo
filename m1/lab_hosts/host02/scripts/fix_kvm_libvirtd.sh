#!/usr/bin/env bash
set -euo pipefail

echo "[host02] applying remediation: start libvirtd + repopulate VM inventory"

cat > /opt/data/hypervisor/kvm_libvirtd.json <<'JSON'
{"Name":"libvirtd","Status":"active","Hypervisor":"KVM"}
JSON

cat > /opt/data/virtual_machine/kvm_virsh_list_all.json <<'JSON'
{"vms":[
  {"UUID":"33333333-3333-3333-3333-333333333333","Name":"sockshop-catalog","State":"running"},
  {"UUID":"44444444-4444-4444-4444-444444444444","Name":"sockshop-orders","State":"running"}
]}
JSON

echo "[host02] remediation done"
