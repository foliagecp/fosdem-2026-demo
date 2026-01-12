#!/usr/bin/env bash
set -euo pipefail

# Emulates: lsmod | <transform-to-json>
cat /opt/data/hypervisor/lsmod.json
