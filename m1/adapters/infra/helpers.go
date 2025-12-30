package main

import (
	"fmt"
	"time"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
)

func adapterUpdateStatus(dbc db.DBSyncClient) {
	t := time.Now()
	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))
	_ = dbc.CMDB.ObjectUpdate(infraRootUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_INFRA)
}

func mustString(j easyjson.JSON, path string) (string, error) {
	s, ok := j.GetByPath(path).AsString()
	if !ok || s == "" {
		return "", fmt.Errorf("missing %s", path)
	}
	return s, nil
}
