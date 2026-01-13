#!/usr/bin/env bash
set -euo pipefail

# Emulates: vagrant global-status --machine-readable (raw text dump)
cat /opt/data/virtual_machine/vagrant_global-status.txt
