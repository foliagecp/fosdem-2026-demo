package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m3/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m3/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

const (
	pushUpdateFoliageFunctionName = "function.connector.jsonfile.push_update"
)

var (
	// natsURL - nats server url
	natsURL string = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

/*
Id // JSON file uuid

Payload:

	json:
		uuid: string // Required. JSON file uuid
		data: string // Optional. JSON file body
*/
func pushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "inspect cannot create db client")
		return
	}

	// Data validation ------------------------------------
	uuid, jsonUUIDExists := ctx.Payload.GetByPath("json.uuid").AsString()
	if !jsonUUIDExists {
		lg.Logln(lg.ErrorLevel, "missing \"json_uuid\" field in the payload")
		return
	}
	if !ctx.Payload.PathExists("json.data") {
		lg.Logln(lg.ErrorLevel, "missing \"json_file\" field in the payload")
		return
	}
	// ----------------------------------------------------

	newJsonFileData := ctx.Payload.GetByPath("json.data").NormalizedClone()

	updateNeeded := true
	if data, err := dbc.CMDB.ObjectRead(uuid); err == nil {
		oldJsonFileData := data.GetByPath("body").NormalizedClone()
		if oldJsonFileData.Equals(newJsonFileData) {
			updateNeeded = false
		}
	}

	if updateNeeded {
		err = dbc.CMDB.ObjectUpdate(uuid, newJsonFileData, true, types.TYPE_FOLIAGE_CONNECTOR_JSON_FILE)
		if err != nil {
			lg.Logf(lg.ErrorLevel, "cannot update object with id=%s", uuid)
			return
		}

		err = dbc.CMDB.ObjectsLinkUpdate(apps.APP_CN_JSON_FILE, uuid, nil, easyjson.NewJSONObject(), true, uuid)
		if err != nil {
			lg.Logf(lg.ErrorLevel, "cannot link connector %s to %s: %v", apps.APP_CN_JSON_FILE, uuid, err)
			return
		}

		connectorUpdateStatus(dbc)
		notifyAdapters(dbc, ctx)
	}
}

func notifyAdapters(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor) {
	getPushUpdateFunction := func(adapterUUID string) (string, bool) {
		if data, err := dbc.CMDB.ObjectRead(adapterUUID); err == nil {
			return data.GetByPath("body.push_update_function").AsString()
		}
		return "", false
	}
	for _, dm := range ctx.Domain.GetWeakClusterDomains() {
		if uuids, err := dbc.Query.JPGQLCtraQuery(ctx.Domain.CreateObjectIDWithDomain(dm, types.TYPE_FOLIAGE_APP_ADAPTER, true), ".*[l:type('__object')]"); err == nil {
			for _, uuid := range uuids {
				if typename, ok := getPushUpdateFunction(uuid); ok {
					ctx.Signal(sfPlugins.AutoSignalSelect, typename, uuid, nil, nil)
				}
			}
		}
	}
}

func connectorUpdateStatus(dbc db.DBSyncClient) {
	t := time.Now()

	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_CN_JSON_FILE, data, false, types.TYPE_FOLIAGE_APP_CONNECTOR))
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, pushUpdateFoliageFunctionName, pushUpdate, *statefun.NewFunctionTypeConfig())
}

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_CONNECTOR, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_CN_JSON_FILE, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_APP_CONNECTOR))

	// Init model data ----------------------------------------------
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_CONNECTOR_JSON_FILE, easyjson.NewJSONObject(), false, true))
	// --------------------------------------------------------------

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_CONNECTOR, types.TYPE_FOLIAGE_CONNECTOR_JSON_FILE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_CONNECTOR_JSON_FILE))

	return nil
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_CN_JSON_FILE).SetDomainRoutersHandling(false).UseJSDomainAsHubDomainName()); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig(apps.APP_CN_JSON_FILE+"cache")); err != nil {
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

// --------------------------------------------------------------------------------------
