package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	statefun "github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

const (
	pushUpdateFnName     = "function.adapter.infra.push_update"
	postProcessingFnName = "function.adapter.infra.post_process"
)

var (
	// Root node ID is kept human-readable to simplify demos.
	infraRootUUID = "infra"
)

var (
	natsURL string = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, pushUpdateFnName, infraPushUpdate, *statefun.NewFunctionTypeConfig())
	statefun.NewFunctionType(runtime, postProcessingFnName, infraPostProcess, *statefun.NewFunctionTypeConfig().SetAllowedSignalProviders(sfPlugins.AutoSignalSelect))
}

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	// Adapter app object.
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, easyjson.NewJSONObject(), false, true))

	adapterBody := easyjson.NewJSONObject()
	adapterBody.SetByPath("push_update_function", easyjson.NewJSON(pushUpdateFnName))
	adapterBody.SetByPath("post_process_function", easyjson.NewJSON(postProcessingFnName))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_INFRA, adapterBody, true, types.TYPE_FOLIAGE_APP_ADAPTER))

	// Domain types.
	domainTypes := []string{
		types.TYPE_FOLIAGE_ADAPTER_INFRA,
		types.TYPE_FOLIAGE_ADAPTER_SERVER,
		types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR,
		types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE,
		types.TYPE_FOLIAGE_ADAPTER_CPU,
		types.TYPE_FOLIAGE_ADAPTER_RAM_STICK,
		types.TYPE_FOLIAGE_ADAPTER_DISK,
		types.TYPE_FOLIAGE_ADAPTER_BIOS,
		types.TYPE_FOLIAGE_ADAPTER_SN,
		types.TYPE_FOLIAGE_ADAPTER_NETWORK_ADAPTER,
	}
	for _, t := range domainTypes {
		system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(t, easyjson.NewJSONObject(), false, true))
		system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, t, nil, easyjson.NewJSONObject(), false, t))
	}

	// K8s Node type for shadow objects from M2 -------
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_NODE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_NODE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_NODE))
	// ------------------------------------------------

	// Domain links (both directions where needed).
	linkTypes := [][2]string{
		{types.TYPE_FOLIAGE_ADAPTER_INFRA, types.TYPE_FOLIAGE_ADAPTER_SERVER},
		{types.TYPE_FOLIAGE_ADAPTER_INFRA, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR},
		{types.TYPE_FOLIAGE_ADAPTER_INFRA, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE},
		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR},
		{types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR, types.TYPE_FOLIAGE_ADAPTER_SERVER},
		{types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE},
		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR},

		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_CPU},
		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_RAM_STICK},
		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_DISK},
		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_BIOS},
		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_SN},
		{types.TYPE_FOLIAGE_ADAPTER_SERVER, types.TYPE_FOLIAGE_ADAPTER_NETWORK_ADAPTER},

		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_CPU},
		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_RAM_STICK},
		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_DISK},
		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_BIOS},
		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_SN},
		{types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, types.TYPE_FOLIAGE_ADAPTER_NETWORK_ADAPTER},
	}
	for _, p := range linkTypes {
		system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(p[0], p[1], nil, easyjson.NewJSONObject(), false, p[1]))
	}

	// Root object.
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(infraRootUUID, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_INFRA))

	go common.HeartBeat(ctx, runtime, infraRootUUID, types.TYPE_FOLIAGE_ADAPTER_INFRA)

	// Set weak cluster domains to connect to M2 ------
	runtime.Domain.SetWeakClusterDomains([]string{"m2", "m3", "m4"})
	// ------------------------------------------------

	return nil
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_AD_INFRA).UseJSDomainAsHubDomainName().SetDomainRoutersHandling(false)); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig(apps.APP_AD_INFRA+"cache")); err != nil {
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
