package main

import (
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

const (
	vagrantGlobalStatusCommand          = "vagrant_global_status"
	vagrantGlobalStatusPushUpdateFnName = "function.connector.virtual_machine.vagrant_global_status.push_update"
)

// vagrantGlobalStatusPushUpdate ingests the raw output of the "vagrant global-status" data source.
func vagrantGlobalStatusPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "vagrant_global_status.push_update: cannot create db client")
		return
	}
	ingestSource(ctx, dbc, vagrantGlobalStatusCommand, types.TYPE_FOLIAGE_CONNECTOR_VAGRANT_GLOBAL_STATUS)
}
