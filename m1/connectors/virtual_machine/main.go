package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	statefun "github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	"github.com/foliagecp/sdk/statefun/system"
)

var (
	natsURL string = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, vagrantGlobalStatusPushUpdateFnName, vagrantGlobalStatusPushUpdate, *statefun.NewFunctionTypeConfig())
	statefun.NewFunctionType(runtime, lshwPushUpdateFnName, lshwPushUpdate, *statefun.NewFunctionTypeConfig())
}

func onAfterStart(_ context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	// App connector root.
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_CONNECTOR, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_CN_VIRTUAL_MACHINE, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_APP_CONNECTOR))

	// Command source types.
	mustTypeInit(dbc, types.TYPE_FOLIAGE_CONNECTOR_VAGRANT_GLOBAL_STATUS)
	mustTypeInit(dbc, types.TYPE_FOLIAGE_CONNECTOR_LSHW)

	return nil
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_CN_VIRTUAL_MACHINE).UseJSDomainAsHubDomainName().SetDomainRoutersHandling(false)); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig(apps.APP_CN_VIRTUAL_MACHINE+"cache")); err != nil {
			lg.Logf(lg.ErrorLevel, "Cannot start due to an error: %s", err)
		}
	} else {
		lg.Logf(lg.ErrorLevel, "Cannot create statefun runtime due to an error: %s", err)
	}
}

func main() {
	helpFlag := flag.Bool("h", false, "Show help message")
	helpFlagAlias := flag.Bool("help", false, "Show help message (alias)")
	logLevelFlag := flag.Int("ll", int(lg.InfoLevel), "Log level (0-6): panic, fatal, error, warn, info, debug, trace")
	logReportCallerFlag := flag.Bool("lrp", false, "Log report caller")

	flag.Parse()

	if *helpFlag || *helpFlagAlias {
		fmt.Println("usage: foliage [option]")
		fmt.Println("Options:")
		flag.PrintDefaults()
		return
	}

	lg.SetDefaultOptions(os.Stdout, lg.LogLevel((4-*logLevelFlag)*4), *logReportCallerFlag)

	start()
}
