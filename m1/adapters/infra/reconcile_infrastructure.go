package main

import (
	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
)

// reconcileInfrastructure updates the infra root object from the raw infrastructure snapshot.
//
// Expected output shape (flexible):
//   {
//     "name": "demo",
//     "environment": "lab",
//     "description": "...",
//     ...
//   }
func reconcileInfrastructure(dbc db.DBSyncClient, raw easyjson.JSON) {
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.infrastructure", raw)

	if name := raw.GetByPath("name").AsStringDefault(""); name != "" {
		data.SetByPath("summary.name", easyjson.NewJSON(name))
	}
	if env := raw.GetByPath("environment").AsStringDefault(""); env != "" {
		data.SetByPath("summary.environment", easyjson.NewJSON(env))
	}
	if desc := raw.GetByPath("description").AsStringDefault(""); desc != "" {
		data.SetByPath("summary.description", easyjson.NewJSON(desc))
	}

	_ = dbc.CMDB.ObjectUpdate(infraRootUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_INFRA)
}
