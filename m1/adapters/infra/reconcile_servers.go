package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/util"
	"github.com/foliagecp/sdk/clients/go/db"
)

// reconcileServers creates/updates server objects from the raw servers snapshot.
//
// Expected output shape (flexible):
//   {"servers": [{"ip": "...", "name": "..."}, ...]}
func reconcileServers(dbc db.DBSyncClient, raw easyjson.JSON) {
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.servers", raw)
	_ = dbc.CMDB.ObjectUpdate(infraRootUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_INFRA)

	var arr []interface{}
	if raw.IsObject() {
		if a, ok := raw.GetByPath("servers").AsArray(); ok {
			arr = a
		} else if a, ok := raw.GetByPath("data.servers").AsArray(); ok {
			arr = a
		}
	} else {
		if a, ok := raw.AsArray(); ok {
			arr = a
		}
	}
	if arr == nil {
		return
	}

	for _, item := range arr {
		server := easyjson.NewJSON(item)
		ip := strings.TrimSpace(server.GetByPath("ip").AsStringDefault(""))
		if ip == "" {
			ip = strings.TrimSpace(server.GetByPath("address").AsStringDefault(""))
		}
		if ip == "" {
			continue
		}
		hostID := util.HostIDFromIP(ip)
		uuid := ensureServer(dbc, hostID)

		upd := easyjson.NewJSONObject()
		upd.SetByPath("sources.servers", server)
		if name := strings.TrimSpace(server.GetByPath("name").AsStringDefault("")); name != "" {
			upd.SetByPath("summary.name", easyjson.NewJSON(name))
		}
		if role := strings.TrimSpace(server.GetByPath("role").AsStringDefault("")); role != "" {
			upd.SetByPath("summary.role", easyjson.NewJSON(role))
		}
		if dc := strings.TrimSpace(server.GetByPath("datacenter").AsStringDefault("")); dc != "" {
			upd.SetByPath("summary.datacenter", easyjson.NewJSON(dc))
		}
		_ = dbc.CMDB.ObjectUpdate(uuid, upd, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
	}
}
