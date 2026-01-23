package common

import (
	"context"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

const TYPE_FOLIAGE_APP_ADAPTER = "foliage-app-adapter"

func PostProcessNotifier(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, payloads ...easyjson.JSON) {
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
			".*[l:type('__object')]"); err == nil {
			for _, uuid := range uuids {
				if typename, ok := getPostProcessFunction(uuid); ok && len(payloads) > 0 {
					for _, payload := range payloads {
						system.MsgOnErrorReturn(ctx.Signal(sfPlugins.AutoSignalSelect, typename, uuid, payload.GetPtr(), nil))
					}
				}
			}
		}
	}
}

func NotifyAdapters(runtime *statefun.Runtime, dbc db.DBSyncClient, payloads ...easyjson.JSON) {
	le := lg.GetLogger()
	logCtx := context.Background()

	getPostProcessFunction := func(adapterUUID string) (string, bool) {
		if data, err := dbc.CMDB.ObjectRead(adapterUUID); err == nil {
			return data.GetByPath("body.post_process_function").AsString()
		}
		return "", false
	}

	for _, dm := range runtime.Domain.GetWeakClusterDomains() {
		if dm == runtime.Domain.Name() {
			continue
		}

		typeID := runtime.Domain.CreateObjectIDWithDomain(dm, TYPE_FOLIAGE_APP_ADAPTER, true)

		uuids, err := dbc.Query.JPGQLCtraQuery(typeID, ".*[l:type('__object')]")
		if err != nil {
			le.Errorf(logCtx, "notifyAdapters: query failed for domain %s: %v", dm, err)
			continue
		}

		for _, uuid := range uuids {
			if typename, ok := getPostProcessFunction(uuid); ok && len(payloads) > 0 {
				for _, pl := range payloads {
					system.MsgOnErrorReturn(runtime.Signal(sfPlugins.AutoSignalSelect, typename, uuid, pl.GetPtr(), nil))
				}
			} else {
				le.Debugf(logCtx, "notifyAdapters: no post_process_function for adapter %s", uuid)
			}
		}
	}
}
