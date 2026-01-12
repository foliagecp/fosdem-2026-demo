package main

import (
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

const (
	lsmodCommand          = "lsmod"
	lsmodPushUpdateFnName = "function.connector.hypervisor.lsmod.push_update"
)

// lsmodPushUpdate ingests the raw output of the "lsmod" data source.
func lsmodPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "lsmod.push_update: cannot create db client")
		return
	}
	ingestSource(ctx, dbc, lsmodCommand, types.TYPE_FOLIAGE_CONNECTOR_LSMOD)
}
