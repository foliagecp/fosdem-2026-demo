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
	hostnameCommand          = "hostname"
	hostnamePushUpdateFnName = "function.connector.server.hostname.push_update"
)

// Payload MUST contain only stdout of the command (already transformed into JSON by the agent).
func hostnamePushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "hostname.push_update: cannot create db client")
		return
	}

	// Agents publish to the connector vertex ID (cn_server) to force sequential
	// processing. Therefore, host identity must be provided in the payload.
	//
	// Expected payload shape:
	//   {"ip":"<host_ip>", "data": <command_stdout_json>}
	hostIP, ok := ctx.Payload.GetByPath("ip").AsString()
	if !ok || strings.TrimSpace(hostIP) == "" {
		lg.Logln(lg.ErrorLevel, "hostname.push_update: missing ip in payload")
		return
	}
	hostID := strings.ReplaceAll(strings.TrimSpace(hostIP), ".", "_")

	data := ctx.Payload.GetByPath("data")
	newData := data.NormalizedClone()
	if !newData.IsObject() {
		// CMDB bodies are expected to be JSON objects for stable querying.
		wrapped := easyjson.NewJSONObject()
		wrapped.SetByPath("data", newData)
		newData = wrapped
	}
	suuid := sourceUUID(hostID, hostnameCommand)

	old, err := dbc.CMDB.ObjectRead(suuid)
	if err == nil {
		if old.GetByPath("body").NormalizedClone().Equals(newData) {
			connectorUpdateStatus(dbc)
			return
		}
	}

	if err := dbc.CMDB.ObjectUpdate(suuid, newData, true, types.TYPE_FOLIAGE_CONNECTOR_HOSTNAME); err != nil {
		lg.Logf(lg.ErrorLevel, "hostname.push_update: cannot update source %s: %v", suuid, err)
		return
	}

	_ = dbc.CMDB.ObjectsLinkUpdate(apps.APP_CN_SERVER, suuid, nil, easyjson.NewJSONObject(), false, hostID)
	connectorUpdateStatus(dbc)

	notifyAdapters(dbc, ctx, hostID, hostnameCommand, suuid)
}
