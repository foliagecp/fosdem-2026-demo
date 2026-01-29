package main

import (
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/sdk/clients/go/db"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

const TYPE_FOLIAGE_APP_ADAPTER = "foliage-app-adapter"

func postProcessNotifier(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor) {
	getPostProcessFunction := func(adapterUUID string) (string, bool) {
		if data, err := dbc.CMDB.ObjectRead(adapterUUID); err == nil {
			return data.GetByPath("body.post_process_function").AsString()
		}
		return "", false
	}
	for _, dm := range ctx.Domain.GetWeakClusterDomains() {
		if dm == ctx.Domain.Name() {
			continue
		}
		if uuids, err := dbc.Query.JPGQLCtraQuery(
			ctx.Domain.CreateObjectIDWithDomain(dm, TYPE_FOLIAGE_APP_ADAPTER, true),
			common.AllObjectsQuery); err == nil {
			for _, uuid := range uuids {
				if typename, ok := getPostProcessFunction(uuid); ok {
					system.MsgOnErrorReturn(ctx.Signal(sfPlugins.AutoSignalSelect, typename, uuid, nil, nil))
				}
			}
		}
	}
}
