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

# Server connector functions.
require_env PUSH_INFRASTRUCTURE_FN
require_env PUSH_SERVERS_FN
require_env PUSH_LSHW_SERVER_FN

# Hypervisor connector functions.
require_env PUSH_LSMOD_FN

# Virtual machine connector functions.
require_env PUSH_VAGRANT_GLOBAL_STATUS_FN
require_env PUSH_LSHW_VM_FN

INTERVAL_SEC="${INTERVAL_SEC:-5}"
BOOTSTRAP="${BOOTSTRAP:-0}"
VERBOSE="${VERBOSE:-0}"

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
  local ip="$3"

  if ! printf '%s' "$raw_json" | jq -e . >/dev/null 2>&1; then
    log "invalid JSON from command, skipping publish to $subject"
    return 1
  fi

  # Build the payload as {ip, data} without any extra metadata.
  # raw_json is already validated as JSON above, so we can safely embed it as `data: .`.
  local payload
  payload="$(printf '%s' "$raw_json" | tr -d '\r' | jq -c --arg ip "$ip" '{ip: $ip, data: .}')"

  # SDK envelope.
  local msg
  msg="$(printf '%s' "$payload" | jq -c '{payload: .}')"

  local len
  len="$(printf '%s' "$msg" | wc -c | tr -d ' ')"

  [[ "$VERBOSE" == "1" ]] && log "pub -> $subject"

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
  local ip_override="$4"

  local out
  if ! out="$(eval "$local_cmd")"; then
    log "local command failed: $local_cmd"
    return 1
  fi

  local subject
  subject="$(signal_subject "$fn_typename" "$vertex_id")"

  local ip
  ip="$HOST_IP"
  if [[ -n "${ip_override:-}" ]]; then
    ip="$ip_override"
  fi

  nats_pub_payload "$subject" "$out" "$ip" || true
}

log "starting (domain=$FO_DOMAIN host_ip=$HOST_IP bootstrap=$BOOTSTRAP)"

bootstrap_once() {
  if [[ "$BOOTSTRAP" != "1" ]]; then
    return 0
  fi
  if [[ -f /tmp/bootstrap_done ]]; then
    return 0
  fi
  log "bootstrapping infrastructure + servers"
  push_once "$PUSH_INFRASTRUCTURE_FN" "$CN_SERVER_ID" "/opt/commands/infrastructure_cmd.sh" "" || true
  push_once "$PUSH_SERVERS_FN" "$CN_SERVER_ID" "/opt/commands/servers_cmd.sh" "" || true
  touch /tmp/bootstrap_done || true
}

push_vms_lshw() {
  local vagrant_json="$1"
  # Expect: {"vms":[{"name":"...","ip":"..."}, ...]}
  local count
  count=$(printf '%s' "$vagrant_json" | jq -r '.vms | length' 2>/dev/null || echo 0)
  if [[ "$count" == "0" ]]; then
    return 0
  fi
  for i in $(seq 0 $((count-1))); do
    local name ip
    name=$(printf '%s' "$vagrant_json" | jq -r ".vms[$i].name" 2>/dev/null || echo "")
    ip=$(printf '%s' "$vagrant_json" | jq -r ".vms[$i].ip" 2>/dev/null || echo "")
    if [[ -z "$name" || -z "$ip" || "$ip" == "null" ]]; then
      continue
    fi
    push_once "$PUSH_LSHW_VM_FN" "$CN_VIRTUAL_MACHINE_ID" "/opt/commands/lshw_vm_cmd.sh \"$name\"" "$ip" || true
  done
}

while true; do
  bootstrap_once || true

  # Server snapshot.
  push_once "$PUSH_LSHW_SERVER_FN" "$CN_SERVER_ID" "/opt/commands/lshw_server_cmd.sh" "" || true

  # Hypervisor snapshot.
  push_once "$PUSH_LSMOD_FN" "$CN_HYPERVISOR_ID" "/opt/commands/lsmod_cmd.sh" "" || true

  # VMs snapshot.
  vagrant_out="$(/opt/commands/vagrant_global_status_cmd.sh 2>/dev/null || echo '{}')"
  subject="$(signal_subject "$PUSH_VAGRANT_GLOBAL_STATUS_FN" "$CN_VIRTUAL_MACHINE_ID")"
  nats_pub_payload "$subject" "$vagrant_out" "$HOST_IP" || true

  push_vms_lshw "$vagrant_out" || true

  sleep "$INTERVAL_SEC"
done
