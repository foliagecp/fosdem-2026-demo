#!/usr/bin/env bash
set -euo pipefail

echo "[host01] applying remediation: start libvirtd + repopulate VM inventory"

cat > /opt/data/hypervisor/kvm_libvirtd.json <<'JSON'
{"Name":"libvirtd","Status":"active","Hypervisor":"KVM"}
JSON

# For host01 we keep the default inventory of two VMs.
cat > /opt/data/virtual_machine/kvm_virsh_list_all.json <<'JSON'
{"vms":[
  {"UUID":"11111111-1111-1111-1111-111111111111","Name":"sockshop-frontend","State":"running"},
  {"UUID":"22222222-2222-2222-2222-222222222222","Name":"sockshop-cart","State":"running"}
]}
JSON

echo "[host01] remediation done"
