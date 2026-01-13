#!/usr/bin/env bash
set -euo pipefail

# Demo remediation script.
# In a real system, this would load required kernel modules, restart services, etc.
# Here we simply replace the emulated "lsmod" snapshot with a "healthy" one.

cat > /opt/data/hypervisor/lsmod.json <<'EOF'
{
  "modules": [
    "kvm",
    "kvm_intel",
    "vhost_net",
    "tap",
    "bridge"
  ]
}
EOF

echo "OK"