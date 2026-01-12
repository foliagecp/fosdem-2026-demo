#!/usr/bin/env bash
set -euo pipefail

# Emulates: vagrant global-status --prune | <transform-to-json>
cat /opt/data/virtual_machine/vagrant_global_status.json
