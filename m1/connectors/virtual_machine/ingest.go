package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/util"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

// ingestSource stores a raw command snapshot as a CMDB object and emits adapter signals.
//
// Expected payload shape:
//
//	{"id":"<host_id>", "data": <command_stdout_json>}
func ingestSource(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, command string, sourceType string) {
	hostID, ok := ctx.Payload.GetByPath("id").AsString()
	if !ok || strings.TrimSpace(hostID) == "" {
		lg.Logf(lg.ErrorLevel, "%s.push_update: missing id in payload", command)
		return
	}
	hostID = util.GetSafeName(hostID)

	data := ctx.Payload.GetByPath("data")
	newData := data.NormalizedClone()
	if !newData.IsObject() {
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
