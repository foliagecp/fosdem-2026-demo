#!/usr/bin/env bash
set -euo pipefail

# Ensure host keys exist (some base images don't generate them by default).
ssh-keygen -A >/dev/null 2>&1 || true

# Allow the demo SSH user to mutate emulated snapshots under /opt/data (remediation demos).
chmod -R a+rwX /opt/data >/dev/null 2>&1 || true

# Start sshd for demo remediation calls.
/usr/sbin/sshd

# Start the inventory agent in background.
/opt/agent/agent.sh &

# Keep container running.
tail -f /dev/null
