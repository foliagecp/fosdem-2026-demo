package main

import (
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

const (
	lshwCommand          = "lshw"
	lshwPushUpdateFnName = "function.connector.server.lshw.push_update"
)

// lshwPushUpdate ingests the raw output of the "lshw" data source for a server.
// Payload MUST contain only stdout of the command (already transformed into JSON by the agent).
func lshwPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "lshw.push_update: cannot create db client")
		return
	}
	ingestSource(ctx, dbc, lshwCommand, types.TYPE_FOLIAGE_CONNECTOR_LSHW)
}
