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
	statusIsReadyTmpl              = "%s_status_IsReady"
)

var (
	// natsURL - nats server url
	natsURL           = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
	heartbeatInterval = system.GetEnvMustProceed("HEARTBEAT_INTERVAL_SEC", 11)
)

func adapterUpdateStatus(dbc db.DBSyncClient, domain string) {
	t := time.Now()

	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))
	data.SetByPath(fmt.Sprintf(statusIsReadyTmpl, domain), easyjson.NewJSON(true))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(datacenterRootUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))
}

func postProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	payload := ctx.Payload
	if operation := payload.GetByPath("operation").AsStringDefault(""); operation != "link_model" {
		//lg.Logf(lg.InfoLevel, "skip post process operation '%s'", operation)
		return
	}

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "cannot create db client")
		return
	}

	id, ok := payload.GetByPath("id").AsString()
	if !ok {
		lg.Logf(lg.ErrorLevel, "cannot get id from payload")
		return
	}

	weakDomain, ok := payload.GetByPath("domain").AsString()
	if !ok {
		lg.Logf(lg.ErrorLevel, "cannot get domain from object: %s", id)
		return
	}

	if weakDomain != ctx.Domain.Name() {
		shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), weakDomain, id)

		objType, ok := payload.GetByPath("type").AsString()
		if !ok {
			lg.Logf(lg.WarnLevel, "cannot get domain from object: %s", id)
			return
		}

		dbc.CMDB.ShadowObjectCanBeRecevier = true
		system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, objType))
		dbc.CMDB.ShadowObjectCanBeRecevier = false

		if err = dbc.CMDB.ObjectsLinkUpdate(datacenterRootUUID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID); err != nil {
			lg.Logf(lg.InfoLevel, "Linked datacenter to shadow model root object: %s -> %s", datacenterRootUUID, shadowID)
		}
	}

	adapterUpdateStatus(dbc, weakDomain)
}

func heartbeat(ctx context.Context, runtime *statefun.Runtime) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "cannot create db client")
		return
	}
	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			datacenter, err := dbc.CMDB.ObjectRead(datacenterRootUUID)
			if err != nil {
				lg.Logln(lg.ErrorLevel, "cannot read datacenter from db")
				continue
			}

			outLinks := datacenter.GetByPath("links.out.ids")
			for i := 0; i < outLinks.ArraySize(); i++ {
				linkType := outLinks.ArrayElement(i).AsStringDefault("")
				if runtime.Domain.GetObjectIDWithoutDomain(linkType) != types.TYPE_FOLIAGE_ADAPTER_DATACENTER {
					linkName, ok := outLinks.ArrayElement(i).AsString()
					if !ok {
						lg.Logf(lg.ErrorLevel, "cannot get link name for object: %s", outLinks.ArrayElement(i))
						continue
					}
					domain, id, err := runtime.Domain.GetShadowObjectDomainAndID(linkName)
					if err != nil {
						lg.Logf(lg.ErrorLevel, "cannot get shadow object domain for object: %s", outLinks.ArrayElement(i))
						continue
					}
					idForCheck := runtime.Domain.CreateObjectIDWithDomain(domain, id, true)
					_, err = dbc.CMDB.ObjectRead(idForCheck)
					if err != nil {
						lg.Logf(lg.WarnLevel, "::::::model '%s' is not available from %s", domain, runtime.Domain.Name())
						payload := easyjson.NewJSONObjectWithKeyValue(fmt.Sprintf(statusIsReadyTmpl, domain), easyjson.NewJSON(false))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(datacenterRootUUID, payload, false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectDelete(linkType))
					}
				}
			}
		}
	}
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, postProcessFoliageFunctionName, postProcess, *statefun.NewFunctionTypeConfig())
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

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(datacenterRootUUID, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_DATACENTER))

	// --- Link to leaf models ---
	runtime.Domain.SetWeakClusterDomains([]string{"m1", "m2", "m3"})
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_INFRA, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_ADAPTER_INFRA, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_INFRA))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_DATACENTER, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL))
	///

	go heartbeat(ctx, runtime)

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
