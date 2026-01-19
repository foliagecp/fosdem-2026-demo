package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m4/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m4/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

const (
	pushUpdateFoliageFunctionName = "function.adapter.dc.push_update"
)

var (
	// natsURL - nats server url
	natsURL = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func adapterUpdateStatus(dbc db.DBSyncClient) {
	t := time.Now()

	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_DC, data, false, types.TYPE_FOLIAGE_APP_ADAPTER))
}

func pushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "cannot create db client")
		return
	}

	dcObjectID := system.GetHashStr(types.TYPE_FOLIAGE_ADAPTER_DATACENTER)

	for _, domain := range ctx.Domain.GetWeakClusterDomains() {
		if domain == ctx.Domain.HubDomainName() {
			continue
		}
		adapterTypeID := ctx.Domain.CreateObjectIDWithDomain(domain, types.TYPE_FOLIAGE_APP_ADAPTER, true)

		adapterUUIDs, err := dbc.Query.JPGQLCtraQuery(adapterTypeID, ".*[l:type('__object')]")
		if err != nil {
			lg.Logf(lg.WarnLevel, "Failed to find adapters in domain %s: %v", domain, err)
			continue
		}

		if len(adapterUUIDs) == 0 {
			lg.Logf(lg.WarnLevel, "No adapters found in domain %s", domain)
			continue
		}

		lg.Logf(lg.InfoLevel, "Found %d adapter(s) in domain %s", len(adapterUUIDs), domain)

		for _, adapter := range adapterUUIDs {
			objData, err := dbc.CMDB.ObjectRead(adapter)
			if err != nil {
				lg.Logf(lg.WarnLevel, "Failed to read object %s: %v", adapter, err)
				continue
			}

			objType, ok := objData.GetByPath("type").AsString()
			if !ok {
				lg.Logf(lg.WarnLevel, "Object %s has no type", adapter)
				continue
			}

			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), domain, ctx.Domain.GetObjectIDWithoutDomain(adapter))

			dbc.CMDB.ShadowObjectCanBeRecevier = true
			system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, objType))
			dbc.CMDB.ShadowObjectCanBeRecevier = false

			system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(dcObjectID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID))
			lg.Logf(lg.InfoLevel, "Linked datacenter to shadow: %s -> %s", dcObjectID, shadowID)
		}
	}

	adapterUpdateStatus(dbc)
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, pushUpdateFoliageFunctionName, pushUpdate, *statefun.NewFunctionTypeConfig())
}

func onAfterStart(_ context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, easyjson.NewJSONObject(), false, true))
	adapterBody := easyjson.NewJSONObjectWithKeyValue("push_update_function", easyjson.NewJSON(pushUpdateFoliageFunctionName))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_DC, adapterBody, false, types.TYPE_FOLIAGE_APP_ADAPTER))

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, types.TYPE_FOLIAGE_ADAPTER_DATACENTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_APP_ADAPTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_APP_ADAPTER))

	dcObjectID := system.GetHashStr(types.TYPE_FOLIAGE_ADAPTER_DATACENTER)
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(dcObjectID, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))

	runtime.Domain.SetWeakClusterDomains([]string{"m1", "m2", "m3"})

	return nil
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_AD_DC).SetDomainRoutersHandling(false).UseJSDomainAsHubDomainName()); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, true)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig(apps.APP_AD_DC+"cache")); err != nil {
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
	logReportCallerFlag := flag.Bool("lrp", false, "Log report caller shows file name and line number where log originates from")

	flag.Parse()

	if *helpFlag || *helpFlagAlias {
		fmt.Println("usage: foliage [option]")
		fmt.Println("Options:")
		flag.PrintDefaults()
		return
	}

	lg.SetDefaultOptions(
		os.Stdout,
		// subtract and multiply, because each level has a factor of 4: -8, -4, 0, 4, 8, 12, 16
		lg.LogLevel((4-*logLevelFlag)*4),
		*logReportCallerFlag,
	)

	start()
}
