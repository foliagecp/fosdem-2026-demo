package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

// ingestSource stores a raw command snapshot as a CMDB object and emits adapter signals.
//
// Agents publish to the connector vertex IDs (cn_server/cn_hypervisor/cn_virtual_machine) to ensure
// sequential processing per connector. Therefore, host identity must be provided in the payload.
//
// Expected payload shape:
//   {"ip":"<host_ip>", "data": <command_stdout_json>}
func ingestSource(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, command string, sourceType string) {
	hostIP, ok := ctx.Payload.GetByPath("ip").AsString()
	if !ok || strings.TrimSpace(hostIP) == "" {
		lg.Logf(lg.ErrorLevel, "%s.push_update: missing ip in payload", command)
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

	suuid := sourceUUID(hostID, command)
	old, err := dbc.CMDB.ObjectRead(suuid)
	if err == nil {
		if old.GetByPath("body").NormalizedClone().Equals(newData) {
			connectorUpdateStatus(dbc)
			return
		}
	}

	if err := dbc.CMDB.ObjectUpdate(suuid, newData, true, sourceType); err != nil {
		lg.Logf(lg.ErrorLevel, "%s.push_update: cannot update source %s: %v", command, suuid, err)
		return
	}

	_ = dbc.CMDB.ObjectsLinkUpdate(ctx.Domain.GetObjectIDWithoutDomain(ctx.Self.ID), suuid, nil, easyjson.NewJSONObject(), false, hostID)
	connectorUpdateStatus(dbc)

	notifyAdapters(dbc, ctx, hostID, command, suuid, sourceType)
}
