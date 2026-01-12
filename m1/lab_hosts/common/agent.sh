#!/usr/bin/env bash
set -euo pipefail

# Demo inventory agent that runs ON the server it inventories.
#
# The agent periodically executes local "command" scripts under /opt/commands
# and publishes the raw command output into Foliage connector functions via
# NATS Core (PUB). This mirrors a real deployment where the agent is installed
# on each physical host and is triggered by cron/systemd timers.
#
# IMPORTANT: the agent publishes a payload of the following shape:
#   { "ip": "<host ip>", "data": <command stdout json> }
# and then wraps it into SDK envelope:
#   { "payload": { ... } }
#
# No additional metadata is added.

log() {
  echo "[$(date -Iseconds)] [lab-agent] $*" >&2
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    log "missing env var: $name"
    exit 1
  fi
}

require_env NATS_HOST
require_env NATS_PORT
require_env NATS_USER
require_env NATS_PASS
require_env FO_DOMAIN
require_env HOST_IP

# Connector vertex IDs. Agents publish signals to connector IDs so that each
# connector processes updates sequentially (single vertex), which is useful for demos.
require_env CN_SERVER_ID
require_env CN_HYPERVISOR_ID
require_env CN_VIRTUAL_MACHINE_ID

require_env PUSH_HOSTNAME_FN
require_env PUSH_LIBVIRTD_FN
require_env PUSH_VIRSH_LIST_ALL_FN

INTERVAL_SEC="${INTERVAL_SEC:-5}"

signal_subject() {
  local fn_typename="$1"
  local vertex_id="$2"
  echo "signal.${FO_DOMAIN}.${fn_typename}.${vertex_id}"
}

# Publish a JSON payload to a NATS subject using the Core NATS protocol.
# Message payload is wrapped in {"payload": ...} per SDK expectations.
nats_pub_payload() {
  local subject="$1"
  local raw_json="$2"

  if ! printf '%s' "$raw_json" | jq -e . >/dev/null 2>&1; then
    log "invalid JSON from command, skipping publish to $subject"
    return 1
  fi

  # Build the payload as {ip, data} without any extra metadata.
  # raw_json is already validated as JSON above, so we can safely embed it as `data: .`.
  local payload
  payload="$(printf '%s' "$raw_json" | tr -d '\r' | jq -c --arg ip "$HOST_IP" '{ip: $ip, data: .}')"

  # SDK envelope.
  local msg
  msg="$(printf '%s' "$payload" | jq -c '{payload: .}')"

  local len
  len="$(printf '%s' "$msg" | wc -c | tr -d ' ')"

  {
    printf 'CONNECT {"verbose":false,"pedantic":false,"tls_required":false,"name":"m1-lab-agent","user":"%s","pass":"%s"}\r\n' "$NATS_USER" "$NATS_PASS"
    printf 'PUB %s %s\r\n' "$subject" "$len"
    printf '%s\r\n' "$msg"
    printf 'QUIT\r\n'
  } | nc -w 2 "$NATS_HOST" "$NATS_PORT" >/dev/null 2>&1 || true
}

push_once() {
  local fn_typename="$1"
  local vertex_id="$2"
  local local_cmd="$3"

  local out
  if ! out="$($local_cmd)"; then
    log "local command failed: $local_cmd"
    return 1
  fi

  local subject
  subject="$(signal_subject "$fn_typename" "$vertex_id")"

  nats_pub_payload "$subject" "$out" || true
}

log "starting (domain=$FO_DOMAIN host_ip=$HOST_IP)"

while true; do
  push_once "$PUSH_HOSTNAME_FN" "$CN_SERVER_ID" "/opt/commands/hostname_cmd.sh" || true
  push_once "$PUSH_LIBVIRTD_FN" "$CN_HYPERVISOR_ID" "/opt/commands/kvm_libvirtd_cmd.sh" || true
  push_once "$PUSH_VIRSH_LIST_ALL_FN" "$CN_VIRTUAL_MACHINE_ID" "/opt/commands/kvm_virsh_list_all_cmd.sh" || true
  sleep "$INTERVAL_SEC"
done
