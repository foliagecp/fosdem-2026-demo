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
	"github.com/foliagecp/sdk/statefun/system"
)

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
	// attach raw source to server/hypervisor/vm depending on command
	switch {
	case sourceType == types.TYPE_FOLIAGE_CONNECTOR_HOSTNAME || command == "hostname":
		reconcileHostname(dbc, serverUUID, raw)

	case sourceType == types.TYPE_FOLIAGE_CONNECTOR_HYPERV_VMMS || command == "hyperv_vmms":
		hypUUID := ensureHypervisor(dbc, serverUUID, hostID)
		reconcileHypervVMMS(dbc, hypUUID, raw)

	case sourceType == types.TYPE_FOLIAGE_CONNECTOR_HYPERV_GET_VM || command == "hyperv_get_vm":
		hypUUID := ensureHypervisor(dbc, serverUUID, hostID)
		reconcileHypervGetVM(dbc, hypUUID, hostID, raw)

	default:
		lg.Logf(lg.WarnLevel, "infra.push_update: unknown command=%s source_type=%s", command, sourceType)
	}

	// keep some top-level linkage + minimal fields
	ip := util.IPFromHostID(hostID)
	_tmp := easyjson.NewJSONObject()
	_tmp.SetByPath("summary.ip", easyjson.NewJSON(ip))
	_ = dbc.CMDB.ObjectUpdate(serverUUID, _tmp, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
	_ = dbc.CMDB.ObjectsLinkUpdate(infraRootUUID, serverUUID, nil, easyjson.NewJSONObject(), false, hostID)

	adapterUpdateStatus(dbc)
}

func ensureServer(dbc db.DBSyncClient, hostID string) string {
	serverUUID := system.GetHashStr(strings.TrimSpace(hostID) + types.TYPE_FOLIAGE_ADAPTER_SERVER)
	if serverUUID == "" {
		serverUUID = "unknown_host"
	}
	data := easyjson.NewJSONObject()
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(serverUUID))
	data.SetByPath("summary.ip", easyjson.NewJSON(util.IPFromHostID(serverUUID)))
	_ = dbc.CMDB.ObjectUpdate(serverUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
	return serverUUID
}

func ensureHypervisor(dbc db.DBSyncClient, serverUUID, hostID string) string {
	hypUUID := system.GetHashStr(fmt.Sprintf("%s__hyperv", hostID) + types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
	data := easyjson.NewJSONObject()
	data.SetByPath("summary.kind", easyjson.NewJSON("Hyper-V"))
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
	_ = dbc.CMDB.ObjectsLinkUpdate(serverUUID, hypUUID, nil, easyjson.NewJSONObject(), false, hypUUID)
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

func reconcileHypervVMMS(dbc db.DBSyncClient, hypUUID string, raw easyjson.JSON) {
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.hyperv_vmms", raw)
	if st := raw.GetByPath("Status").AsStringDefault(""); st != "" {
		data.SetByPath("summary.status", easyjson.NewJSON(st))
	}
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}

func reconcileHypervGetVM(dbc db.DBSyncClient, hypUUID, hostID string, raw easyjson.JSON) {
	// The agent should send only the command output.
	// For demo purposes we support two shapes:
	//   1) {"vms": [...]}
	//   2) [...] (legacy)
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
		lg.Logln(lg.WarnLevel, "infra.push_update: hyperv_get_vm raw has no vms array")
		return
	}

	desired := map[string]easyjson.JSON{}
	for _, item := range arr {
		vm := easyjson.NewJSON(item)
		vmID := vm.GetByPath("VMId").AsStringDefault("")
		if vmID == "" {
			vmID = vm.GetByPath("Id").AsStringDefault("")
		}
		if vmID == "" {
			continue
		}
		vmUUID := sanitizeVMUUID(hostID, vmID)
		desired[vmUUID] = vm
	}

	// delete stale
	if uuids, err := dbc.Query.JPGQLCtraQuery(hypUUID, fmt.Sprintf(".*[l:type('%s')]", types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)); err == nil {
		for _, u := range uuids {
			if _, ok := desired[u]; !ok {
				_ = dbc.CMDB.ObjectDelete(u)
			}
		}
	}

	for vmUUID, vm := range desired {
		data := easyjson.NewJSONObject()
		data.SetByPath("sources.hyperv_get_vm", vm)
		if name := vm.GetByPath("Name").AsStringDefault(""); name != "" {
			data.SetByPath("summary.name", easyjson.NewJSON(name))
		}
		if state := vm.GetByPath("State").AsStringDefault(""); state != "" {
			data.SetByPath("summary.state", easyjson.NewJSON(state))
		}
		data.SetByPath("summary.vm_id", easyjson.NewJSON(vm.GetByPath("VMId").AsStringDefault(vm.GetByPath("Id").AsStringDefault(""))))
		_ = dbc.CMDB.ObjectUpdate(vmUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)
		_ = dbc.CMDB.ObjectsLinkUpdate(hypUUID, vmUUID, nil, easyjson.NewJSONObject(), false, vm.GetByPath("Name").AsStringDefault("vm"))
	}

	// also keep raw at hypervisor level for debugging
	_tmp2 := easyjson.NewJSONObject()
	_tmp2.SetByPath("sources.hyperv_get_vm", raw)
	_ = dbc.CMDB.ObjectUpdate(hypUUID, _tmp2, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}

func sanitizeVMUUID(hostID, vmID string) string {
	vmID = strings.TrimSpace(vmID)
	vmID = strings.ToLower(vmID)
	vmID = strings.ReplaceAll(vmID, "-", "_")
	return system.GetHashStr(fmt.Sprintf("%s__vm__%s", hostID, vmID) + types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)
}
