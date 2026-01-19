#!/usr/bin/env bash
set -euo pipefail

# m1 inventory agent (one agent per bare-metal server).
#
# Responsibilities (per requirements):
#  - Execute local discovery commands:
#      1) lsmod                       (raw text dump -> JSON before publish)
#      2) lshw -json                  (already JSON)
#      3) vagrant global-status -mr   (raw text dump -> JSON before publish)
#  - For each VM found via vagrant global-status:
#      - emulate "vagrant ssh <VM_ID> && sudo -i && lshw -json"
#        (in demo: read /opt/data/virtual_machine/lshw/<VM_ID>.json)
#      - publish VM lshw as JSON wrapper into the SAME lshw connector.
#
# Connector payload shape MUST be:
#   { "ip": "<server ip>", "data": <json> }
# and then wrapped into SDK envelope:
#   { "payload": { ... } }

log() { echo "[$(date -Iseconds)] [lab-agent] $*" >&2; }

: "${FO_DOMAIN:?FO_DOMAIN is required}"
if command -v ip >/dev/null 2>&1; then
  HOST_IP=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}')
fi
: "${HOST_IP:=$(hostname -I 2>/dev/null | awk '{print $1}')}"
: "${NATS_HOST:=nats}"
: "${NATS_PORT:=4222}"
: "${NATS_USER:=nats}"
: "${NATS_PASS:=foliage}"
: "${INTERVAL_SEC:=10}"
: "${VERBOSE:=0}"

: "${CN_SERVER_ID:=cn_server}"
: "${CN_HYPERVISOR_ID:=cn_hypervisor}"
: "${CN_VIRTUAL_MACHINE_ID:=cn_virtual_machine}"

: "${PUSH_LSHW_SERVER_FN:?PUSH_LSHW_SERVER_FN is required}"
: "${PUSH_LSMOD_FN:?PUSH_LSMOD_FN is required}"
: "${PUSH_LSHW_VM_FN:?PUSH_LSHW_VM_FN is required}"
: "${PUSH_VAGRANT_GLOBAL_STATUS_FN:?PUSH_VAGRANT_GLOBAL_STATUS_FN is required}"

signal_subject() {
  local fn_typename="$1"
  local vertex_id="$2"
  echo "signal.${FO_DOMAIN}.${fn_typename}.${vertex_id}"
}

nats_pub_payload() {
  local subject="$1"
  local raw_json="$2"
  local id="$3"

  if ! printf '%s' "$raw_json" | jq -e . >/dev/null 2>&1; then
    log "invalid JSON, skipping publish to $subject"
    return 1
  fi

  local payload msg len
  payload="$(printf '%s' "$raw_json" | tr -d '\r' | jq -c --arg id "$id" '{id: $id, data: .}')"
  msg="$(printf '%s' "$payload" | jq -c '{payload: .}')"
  len="$(printf '%s' "$msg" | wc -c | tr -d ' ')"

  [[ "$VERBOSE" == "1" ]] && log "pub -> $subject"

  {
    printf 'CONNECT {"verbose":false,"pedantic":false,"tls_required":false,"name":"lab-agent","user":"%s","pass":"%s"}\r\n' "$NATS_USER" "$NATS_PASS"
    printf 'PUB %s %s\r\n' "$subject" "$len"
    printf '%s\r\n' "$msg"
#    printf 'QUIT\r\n'
  } | nc -w 2 "$NATS_HOST" "$NATS_PORT" >/dev/null 2>&1 || true
}

transform_lsmod_to_json() {
  # Input: raw `lsmod` text (exactly like the reference dump).
  # Output: {"modules":[{name,size,used,used_by:[...]}, ...]}
  local raw="$1"
  printf '%s' "$raw" | jq -Rs '
    split("\n")
    | .[1:]                                     # skip header
    | map(gsub("\r";"") | gsub("^ +| +$";""))
    | map(select(length>0))
    | map(gsub(" +";" ") | split(" ") | map(select(length>0)))
    | map({
        name: .[0],
        size: (.[1] | tonumber?),
        used: (.[2] | tonumber?),
        used_by: (if (length>3 and (.[3] != "-")) then (.[3] | split(",") | map(select(length>0))) else [] end)
      })
    | {modules: .}
  '
}

transform_vagrant_global_status_to_json() {
  # Input: raw `vagrant global-status --machine-readable` text (exactly like the reference dump).
  # Output:
  #   {
  #     "metadata":{"machine_count":N},
  #     "vms":[{"id":"...","provider":"...","home":"...","name":"...","state":"..."}]
  #   }
  local raw="$1"
  printf '%s' "$raw" | jq -Rs '
    def clean: gsub("\r";"") | select(length>0);
    def parts: split("\n") | map(clean) | map(split(","));
    parts as $rows
    | reduce $rows[] as $r
        ({metadata:{}, vms:[], _cur:null};
          ($r[2] // "") as $k
          | if $k=="metadata" and (($r[3]//"")=="machine-count") then
              .metadata.machine_count = (($r[4]//"0")|tonumber)
            elif $k=="machine-id" then
              (if ._cur != null then .vms += [._cur] else . end)
              | ._cur = {id: ($r[3]//""), provider:"", home:"", name:"", state:""}
            elif $k=="provider-name" then
              ._cur.provider = ($r[3]//"")
            elif $k=="machine-home" then
              ._cur.home = ($r[3]//"")
              | ._cur.name = ((($r[3]//"") | split("/") | last) // "")
            elif $k=="state" then
              ._cur.state = ($r[3]//"")
            else .
            end
        )
    | (if ._cur != null then .vms += [._cur] else . end)
    | del(._cur)
  '
}

publish_json_to_connector() {
  local fn_typename="$1"
  local vertex_id="$2"
  local json_payload="$3"
  local id="$4"
  local subject
  subject="$(signal_subject "$fn_typename" "$vertex_id")"
  nats_pub_payload "$subject" "$json_payload" "$id" || true
}

push_lsmod() {
  local raw json
  raw="$(/opt/commands/lsmod_cmd.sh 2>/dev/null || true)"
  json="$(transform_lsmod_to_json "$raw" 2>/dev/null || echo '{}')"
  publish_json_to_connector "$PUSH_LSMOD_FN" "$CN_HYPERVISOR_ID" "$json" "$HOST_IP"
}

push_lshw_server() {
  local out
  out="$(/opt/commands/lshw_server_cmd.sh 2>/dev/null || echo '{}')"
  publish_json_to_connector "$PUSH_LSHW_SERVER_FN" "$CN_SERVER_ID" "$out" "$HOST_IP"
}

push_vagrant_status() {
  local raw json
  raw="$(/opt/commands/vagrant_global_status_cmd.sh 2>/dev/null || true)"
  json="$(transform_vagrant_global_status_to_json "$raw" 2>/dev/null || echo '{}')"
  publish_json_to_connector "$PUSH_VAGRANT_GLOBAL_STATUS_FN" "$CN_VIRTUAL_MACHINE_ID" "$json" "$HOST_IP"
  printf '%s' "$json"
}

push_lshw_vms() {
  local vagrant_json="$1"
  local count
  count="$(printf '%s' "$vagrant_json" | jq -r '.vms|length' 2>/dev/null || echo 0)"
  if [[ "$count" == "0" ]]; then
    return 0
  fi

  for i in $(seq 0 $((count-1))); do
    local vm_id vm_name lshw wrapper
    vm_id="$(printf '%s' "$vagrant_json" | jq -r ".vms[$i].id" 2>/dev/null || echo "")"
    vm_name="$(printf '%s' "$vagrant_json" | jq -r ".vms[$i].name" 2>/dev/null || echo "")"
    if [[ -z "$vm_id" || "$vm_id" == "null" ]]; then
      continue
    fi
    lshw="$(/opt/commands/lshw_vm_cmd.sh "$vm_id" 2>/dev/null || echo '{}')"
    publish_json_to_connector "$PUSH_LSHW_VM_FN" "$CN_VIRTUAL_MACHINE_ID" "$lshw" "$vm_id"
  done
}

log "starting agent on $HOST_IP (domain=$FO_DOMAIN)"

while true; do
  # (2) lshw on server
  push_lshw_server || true

  # (1) lsmod on server
  push_lsmod || true

  # (3) vagrant global-status on server
  vagrant_json="$(push_vagrant_status || echo '{}')"

  # lshw on each VM (derived from vagrant global-status)
  push_lshw_vms "$vagrant_json" || true

  sleep "$INTERVAL_SEC"
done
