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
	"k8s.io/utils/strings/slices"
)

const (
	postProcessFoliageFunctionName = "function.adapter.dc.post_process"
	datacenterRootUUID             = "datacenter"
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

func postProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	payload := ctx.Payload
	if operation := payload.GetByPath("operation").AsStringDefault(""); operation != "link_model" {
		lg.Logf(lg.InfoLevel, "skip post process operation '%s'", operation)
		return
	}

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "cannot create db client")
		return
	}

	dcObjectID := ctx.Domain.CreateObjectIDWithHubDomain(datacenterRootUUID, false)

	id, ok := payload.GetByPath("id").AsString()
	if !ok {
		lg.Logln(lg.ErrorLevel, "cannot get id from payload")
		return
	}

	weakDomain, ok := payload.GetByPath("domain").AsString()
	if !ok {
		lg.Logln(lg.ErrorLevel, "cannot get id from payload")
		return
	}

	if !slices.Contains(ctx.Domain.GetWeakClusterDomains(), weakDomain) {
		wcd := ctx.Domain.GetWeakClusterDomains()
		wcd = append(wcd, weakDomain)
		ctx.Domain.SetWeakClusterDomains(wcd)
	}

	shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), weakDomain, id)

	objType, ok := payload.GetByPath("type").AsString()
	if !ok {
		lg.Logf(lg.WarnLevel, "Object %s has no type", id)
		return
	}

	dbc.CMDB.ShadowObjectCanBeRecevier = true
	system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, objType))
	dbc.CMDB.ShadowObjectCanBeRecevier = false

	system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(dcObjectID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID))
	lg.Logf(lg.InfoLevel, "Linked datacenter to shadow: %s -> %s", dcObjectID, shadowID)

	adapterUpdateStatus(dbc)
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, postProcessFoliageFunctionName, postProcess, *statefun.NewFunctionTypeConfig())
}

func onAfterStart(_ context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, easyjson.NewJSONObject(), false, true))
	adapterBody := easyjson.NewJSONObjectWithKeyValue("push_update_function", easyjson.NewJSON(postProcessFoliageFunctionName))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_DC, adapterBody, false, types.TYPE_FOLIAGE_APP_ADAPTER))

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, types.TYPE_FOLIAGE_ADAPTER_DATACENTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_APP_ADAPTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_APP_ADAPTER))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(datacenterRootUUID, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))

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
