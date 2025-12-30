#!/usr/bin/env bash
set -euo pipefail

echo "[host02] applying remediation: start vmms + repopulate VM inventory"

cat > /opt/data/hypervisor/hyperv_vmms.json <<'JSON'
{"Name":"vmms","Status":"Running","Hypervisor":"Hyper-V"}
JSON

cat > /opt/data/virtual_machine/hyperv_get_vm.json <<'JSON'
{"vms":[
  {"VMId":"33333333-3333-3333-3333-333333333333","Name":"sockshop-catalog","State":"Running"},
  {"VMId":"44444444-4444-4444-4444-444444444444","Name":"sockshop-orders","State":"Running"}
]}
JSON

echo "[host02] remediation done"
