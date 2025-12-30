package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

const (
	hypervGetVMCommand          = "hyperv_get_vm"
	hypervGetVMPushUpdateFnName = "function.connector.virtual_machine.hyperv_get_vm.push_update"
)

// Payload MUST contain only stdout of the command (already transformed into JSON by the agent).
// For this connector: a JSON array of VM objects.
func hypervGetVMPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "hyperv_get_vm.push_update: cannot create db client")
		return
	}

	// Agents publish to the connector vertex ID (cn_virtual_machine) to force sequential
	// processing. Therefore, host identity must be provided in the payload.
	//
	// Expected payload shape:
	//   {"ip":"<host_ip>", "data": <command_stdout_json>}
	hostIP, ok := ctx.Payload.GetByPath("ip").AsString()
	if !ok || strings.TrimSpace(hostIP) == "" {
		lg.Logln(lg.ErrorLevel, "hyperv_get_vm.push_update: missing ip in payload")
		return
	}
	hostID := strings.ReplaceAll(strings.TrimSpace(hostIP), ".", "_")

	data := ctx.Payload.GetByPath("data")
	newData := data.NormalizedClone()
	if !newData.IsObject() {
		wrapped := easyjson.NewJSONObject()
		wrapped.SetByPath("data", newData)
		newData = wrapped
	}
	suuid := sourceUUID(hostID, hypervGetVMCommand)

	old, err := dbc.CMDB.ObjectRead(suuid)
	if err == nil {
		if old.GetByPath("body").NormalizedClone().Equals(newData) {
			connectorUpdateStatus(dbc)
			return
		}
	}

	if err := dbc.CMDB.ObjectUpdate(suuid, newData, true, types.TYPE_FOLIAGE_CONNECTOR_HYPERV_GET_VM); err != nil {
		lg.Logf(lg.ErrorLevel, "hyperv_get_vm.push_update: cannot update source %s: %v", suuid, err)
		return
	}

	_ = dbc.CMDB.ObjectsLinkUpdate(apps.APP_CN_VIRTUAL_MACHINE, suuid, nil, easyjson.NewJSONObject(), false, hostID)
	connectorUpdateStatus(dbc)

	notifyAdapters(dbc, ctx, hostID, hypervGetVMCommand, suuid)
}
