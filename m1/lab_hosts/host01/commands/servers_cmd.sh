#!/usr/bin/env bash
set -euo pipefail

# Emulates: a "bootstrap" inventory command that returns a list of servers in this environment.
cat /opt/data/bootstrap/servers.json
