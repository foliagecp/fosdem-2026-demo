package main

import (
	"fmt"
	"time"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

func connectorUpdateStatus(dbc db.DBSyncClient) {
	t := time.Now()
	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))
	_ = dbc.CMDB.ObjectUpdate(apps.APP_CN_SERVER, data, false, types.TYPE_FOLIAGE_APP_CONNECTOR)
}

func notifyAdapters(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, hostID, command, sourceUUID string) {
	for _, dm := range ctx.Domain.GetWeakClusterDomains() {
		if uuids, err := dbc.Query.JPGQLCtraQuery(ctx.Domain.CreateObjectIDWithDomain(dm, types.TYPE_FOLIAGE_APP_ADAPTER, true), ".*[l:type('__object')]"); err == nil {
			if err != nil {
				lg.Logf(lg.ErrorLevel, "cannot query adapters at domain %s: %v", dm, err)
				continue
			}

			payload := easyjson.NewJSONObject()
			payload.SetByPath("host_id", easyjson.NewJSON(hostID))
			payload.SetByPath("command", easyjson.NewJSON(command))
			payload.SetByPath("source_uuid", easyjson.NewJSON(sourceUUID))
			payload.SetByPath("source_type", easyjson.NewJSON(types.TYPE_FOLIAGE_CONNECTOR_HOSTNAME))

			for _, uuid := range uuids {
				adapter, err := dbc.CMDB.ObjectRead(uuid)
				if err != nil {
					continue
				}

				fn, ok := adapter.GetByPath("body.push_update_function").AsString()
				if !ok || fn == "" {
					continue
				}

				ctx.Signal(sfPlugins.AutoSignalSelect, fn, uuid, &payload, nil)
			}
		}
	}
}

func sourceUUID(hostID, command string) string {
	// Human-readable and stable.
	return fmt.Sprintf("%s__%s", hostID, command)
}

func mustTypeInit(dbc db.DBSyncClient, typeName string) {
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(typeName, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_CONNECTOR, typeName, nil, easyjson.NewJSONObject(), false, typeName))
}
