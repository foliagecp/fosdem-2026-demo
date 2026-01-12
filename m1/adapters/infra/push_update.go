package main

import (
	"fmt"
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/util"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

// infraPushUpdate is the single entry point for rebuilding the infrastructure digital-twin.
// It is triggered by connectors via ctx.Signal(...) after they ingest a new raw source snapshot.
func infraPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "infra.push_update: cannot create db client")
		return
	}

	// Filter updates by the connector that emitted the signal.
	// Agents publish updates to connector vertex IDs (cn_server/cn_hypervisor/cn_virtual_machine),
	// so ctx.Caller.ID is stable and can be used as a fast routing hint.
	callerID := strings.TrimSpace(ctx.Caller.ID)
	switch ctx.Domain.GetObjectIDWithoutDomain(callerID) {
	case apps.APP_CN_SERVER, apps.APP_CN_HYPERVISOR, apps.APP_CN_VIRTUAL_MACHINE:
		// ok
	default:
		lg.Logf(lg.DebugLevel, "infra.push_update: ignoring signal from caller=%s", callerID)
		return
	}

	payload := ctx.Payload.NormalizedClone()

	hostID, err := mustString(payload, "host_id")
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: %v", err)
		return
	}
	command, err := mustString(payload, "command")
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: %v", err)
		return
	}
	sourceUUID, err := mustString(payload, "source_uuid")
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: %v", err)
		return
	}
	sourceType := payload.GetByPath("source_type").AsStringDefault("")

	srcObj, err := dbc.CMDB.ObjectRead(sourceUUID)
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: cannot read source %s: %v", sourceUUID, err)
		return
	}
	raw := srcObj.GetByPath("body").NormalizedClone()

	serverUUID := ensureServer(dbc, hostID)

	// Attach raw source to server/hypervisor/vm depending on the command.
	switch {
	case sourceType == types.TYPE_FOLIAGE_CONNECTOR_HOSTNAME || command == "hostname":
		reconcileHostname(dbc, serverUUID, raw)

	case sourceType == types.TYPE_FOLIAGE_CONNECTOR_KVM_LIBVIRTD || command == "kvm_libvirtd":
		hypUUID := ensureHypervisor(dbc, serverUUID, hostID)
		reconcileKVMLibvirtd(dbc, hypUUID, raw)

	case sourceType == types.TYPE_FOLIAGE_CONNECTOR_KVM_VIRSH_LIST_ALL || command == "kvm_virsh_list_all":
		hypUUID := ensureHypervisor(dbc, serverUUID, hostID)
		reconcileKVMVirshListAll(dbc, hypUUID, hostID, raw)

	default:
		lg.Logf(lg.WarnLevel, "infra.push_update: unknown command=%s source_type=%s", command, sourceType)
	}

	// Ensure minimal top-level fields + infra -> server linkage.
	ip := util.IPFromHostID(hostID)
	_tmp := easyjson.NewJSONObject()
	_tmp.SetByPath("summary.ip", easyjson.NewJSON(ip))
	_tmp.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(serverUUID, _tmp, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
	_ = dbc.CMDB.ObjectsLinkUpdate(infraRootUUID, serverUUID, nil, easyjson.NewJSONObject(), false, hostID)

	adapterUpdateStatus(dbc)
}

// ensureServer creates/updates the server node for a given host_id.
// host_id is derived from IP and is used as the server UUID to keep object ids readable.
func ensureServer(dbc db.DBSyncClient, hostID string) string {
	hostID = strings.TrimSpace(hostID)
	serverUUID := hostID
	if serverUUID == "" {
		serverUUID = "unknown_host"
	}
	data := easyjson.NewJSONObject()
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	data.SetByPath("summary.ip", easyjson.NewJSON(util.IPFromHostID(hostID)))
	_ = dbc.CMDB.ObjectUpdate(serverUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
	return serverUUID
}

func ensureHypervisor(dbc db.DBSyncClient, serverUUID, hostID string) string {
	hostID = strings.TrimSpace(hostID)
	hypUUID := fmt.Sprintf("%s__kvm", hostID)
	data := easyjson.NewJSONObject()
	data.SetByPath("summary.kind", easyjson.NewJSON("KVM"))
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
	_ = dbc.CMDB.ObjectsLinkUpdate(serverUUID, hypUUID, nil, easyjson.NewJSONObject(), false, "kvm")
	return hypUUID
}

func reconcileHostname(dbc db.DBSyncClient, serverUUID string, raw easyjson.JSON) {
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.hostname", raw)
	if hn := raw.GetByPath("hostname").AsStringDefault(""); hn != "" {
		data.SetByPath("summary.hostname", easyjson.NewJSON(hn))
	}
	_ = dbc.CMDB.ObjectUpdate(serverUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
}

func reconcileKVMLibvirtd(dbc db.DBSyncClient, hypUUID string, raw easyjson.JSON) {
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.kvm_libvirtd", raw)
	if st := raw.GetByPath("Status").AsStringDefault(""); st != "" {
		data.SetByPath("summary.status", easyjson.NewJSON(st))
	}
	if name := raw.GetByPath("Name").AsStringDefault(""); name != "" {
		data.SetByPath("summary.service", easyjson.NewJSON(name))
	}
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}

func reconcileKVMVirshListAll(dbc db.DBSyncClient, hypUUID, hostID string, raw easyjson.JSON) {
	// The agent should send only the command output.
	// Recommended shape:
	//   {"vms": [{"UUID": "...", "Name": "...", "State": "running"}, ...]}
	// Legacy/demo fallbacks:
	//   1) {"data": ...}
	//   2) [...] (array)
	var arr []interface{}
	if raw.IsObject() {
		if a, ok := raw.GetByPath("vms").AsArray(); ok {
			arr = a
		}
	} else {
		if a, ok := raw.AsArray(); ok {
			arr = a
		}
	}
	if arr == nil {
		lg.Logln(lg.WarnLevel, "infra.push_update: kvm_virsh_list_all raw has no vms array")
		return
	}

	desired := map[string]easyjson.JSON{}
	for _, item := range arr {
		vm := easyjson.NewJSON(item)
		vmID := vm.GetByPath("UUID").AsStringDefault("")
		if vmID == "" {
			vmID = vm.GetByPath("Id").AsStringDefault("")
		}
		if vmID == "" {
			continue
		}
		vmUUID := makeVMUUID(hostID, vmID)
		desired[vmUUID] = vm
	}

	// Delete stale VMs linked under this hypervisor.
	if uuids, err := dbc.Query.JPGQLCtraQuery(hypUUID, fmt.Sprintf(".*[l:type('%s')]", types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)); err == nil {
		for _, u := range uuids {
			if _, ok := desired[u]; !ok {
				_ = dbc.CMDB.ObjectDelete(u)
			}
		}
	}

	for vmUUID, vm := range desired {
		data := easyjson.NewJSONObject()
		data.SetByPath("sources.kvm_virsh_list_all", vm)
		if name := vm.GetByPath("Name").AsStringDefault(""); name != "" {
			data.SetByPath("summary.name", easyjson.NewJSON(name))
		}
		if state := vm.GetByPath("State").AsStringDefault(""); state != "" {
			data.SetByPath("summary.state", easyjson.NewJSON(state))
		}
		data.SetByPath("summary.uuid", easyjson.NewJSON(vm.GetByPath("UUID").AsStringDefault(vm.GetByPath("Id").AsStringDefault(""))))
		_ = dbc.CMDB.ObjectUpdate(vmUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)
		_ = dbc.CMDB.ObjectsLinkUpdate(hypUUID, vmUUID, nil, easyjson.NewJSONObject(), false, vm.GetByPath("Name").AsStringDefault("vm"))
	}

	// Also keep raw at hypervisor level for debugging.
	_tmp2 := easyjson.NewJSONObject()
	_tmp2.SetByPath("sources.kvm_virsh_list_all", raw)
	_ = dbc.CMDB.ObjectUpdate(hypUUID, _tmp2, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}

func makeVMUUID(hostID, vmID string) string {
	vmID = strings.TrimSpace(vmID)
	vmID = strings.ToLower(vmID)
	vmID = strings.ReplaceAll(vmID, "-", "_")
	vmID = strings.ReplaceAll(vmID, ":", "_")
	return fmt.Sprintf("%s__vm__%s", strings.TrimSpace(hostID), vmID)
}
