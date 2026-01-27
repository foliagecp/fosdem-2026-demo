package common

import (
	"context"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var heartbeatInterval = system.GetEnvMustProceed("HEARTBEAT_INTERVAL_SEC", 10)

func HeartBeat(ctx context.Context, runtime *statefun.Runtime, id, _type string) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		lg.GetLogger().Errorf(ctx, "heartbeat request failed: %v", err)
		return
	}
	notifierPayload := easyjson.NewJSONObject().GetPtr()
	notifierPayload.SetByPath("domain", easyjson.NewJSON(runtime.Domain.Name()))
	notifierPayload.SetByPath("id", easyjson.NewJSON(id))
	notifierPayload.SetByPath("type", easyjson.NewJSON(_type))
	notifierPayload.SetByPath("operation", easyjson.NewJSON("link_model"))
	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			getPostProcessFunction := func(adapterUUID string) (string, bool) {
				if data, err := dbc.CMDB.ObjectRead(adapterUUID); err == nil {
					return data.GetByPath("body.post_process_function").AsString()
				}
				return "", false
			}
			for _, dm := range runtime.Domain.GetWeakClusterDomains() {
				if uuids, err := dbc.Query.JPGQLCtraQuery(
					runtime.Domain.CreateObjectIDWithDomain(dm, TYPE_FOLIAGE_APP_ADAPTER, true),
					".*[l:type('__object')]"); err == nil {
					for _, uuid := range uuids {
						if typename, ok := getPostProcessFunction(uuid); ok {
							system.MsgOnErrorReturn(runtime.Signal(sfPlugins.AutoSignalSelect, typename, uuid, notifierPayload, nil))
						}
					}
				}
			}
		}
	}
}
