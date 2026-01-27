package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
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
	postProcessFoliageFunctionName = "function.adapter.dc.post_process"
	datacenterRootUUID             = "datacenter"
	linkedModelTag                 = "linked_model"
	statusReadyTmpl                = "%s_status_IsReady"
)

var (
	// natsURL - nats server url
	natsURL = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func adapterUpdateStatus(dbc db.DBSyncClient, data easyjson.JSON) {
	t := time.Now()

	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))
	data.SetByPath("m4_status_IsReady", easyjson.NewJSON(true))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(datacenterRootUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))
}

func datacenterPostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		le.Errorf(logCtx, "datacenterPostProcess: cannot create db client: %v", err)
		return
	}

	query := fmt.Sprintf(".*[l:tags('%s')]", linkedModelTag)
	linkedIDs, err := dbc.Query.JPGQLCtraQuery(datacenterRootUUID, query)
	if err != nil {
		le.Errorf(logCtx, "datacenterPostProcess: cannot query linked models datacenters: %v", err)
	}

	body := easyjson.NewJSONObject()

	for _, linked := range linkedIDs {
		dm, _, _ := ctx.Domain.GetShadowObjectDomainAndID(linked)
		isReady := false
		if _, err := dbc.CMDB.ObjectRead(ctx.Domain.GetObjectIDByShadowObjectID(linked)); err == nil {
			isReady = true
		}
		body.SetByPath(fmt.Sprintf(statusReadyTmpl, dm), easyjson.NewJSON(isReady))
	}

	adapterUpdateStatus(dbc, body)
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, postProcessFoliageFunctionName, datacenterPostProcess, *statefun.NewFunctionTypeConfig())
	statefun.NewFunctionType(runtime, common.PropagateErrorFunctionName, common.PropagateError, *statefun.NewFunctionTypeConfig().SetMultipleInstancesAllowance(true))
}

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, easyjson.NewJSONObject(), false, true))
	adapterBody := easyjson.NewJSONObjectWithKeyValue("post_process_function", easyjson.NewJSON(postProcessFoliageFunctionName))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_DC, adapterBody, false, types.TYPE_FOLIAGE_APP_ADAPTER))

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, types.TYPE_FOLIAGE_ADAPTER_DATACENTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_APP_ADAPTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_APP_ADAPTER))

	datacenterBody := easyjson.NewJSONObject()
	for _, domain := range runtime.Domain.GetWeakClusterDomains() {
		datacenterBody.SetByPath(fmt.Sprintf(statusReadyTmpl, domain), easyjson.NewJSON(false))
	}
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(datacenterRootUUID, datacenterBody, false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))

	connectLeafModel := func(rootID, _type, domain string) {
		dbc.CMDB.ShadowObjectCanBeRecevier = true
		shadowID := runtime.Domain.CreateCustomShadowId(runtime.Domain.HubDomainName(), domain, rootID)
		body := easyjson.NewJSONObject()
		system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(shadowID, body, false, _type))
		system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(datacenterRootUUID, shadowID, []string{linkedModelTag}, body, false, _type))
		dbc.CMDB.ShadowObjectCanBeRecevier = false
	}

	// --- Link to leaf models ---
	runtime.Domain.SetWeakClusterDomains([]string{common.ModelM1, common.ModelM2, common.ModelM3})
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_INFRA, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_ADAPTER_INFRA, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_INFRA))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL))

	connectLeafModel(common.M1RootObject, types.TYPE_FOLIAGE_ADAPTER_INFRA, common.ModelM1)
	connectLeafModel(common.M2RootObject, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, common.ModelM2)
	connectLeafModel(common.M3RootObject, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, common.ModelM3)
	///

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
