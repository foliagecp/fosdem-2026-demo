package main

import (
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

const (
	serversCommand          = "servers"
	serversPushUpdateFnName = "function.connector.server.servers.push_update"
)

// serversPushUpdate ingests the raw output of the "servers" data source.
// Payload MUST contain only stdout of the command (already transformed into JSON by the agent).
func serversPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "servers.push_update: cannot create db client")
		return
	}
	ingestSource(ctx, dbc, serversCommand, types.TYPE_FOLIAGE_CONNECTOR_SERVERS)
}
