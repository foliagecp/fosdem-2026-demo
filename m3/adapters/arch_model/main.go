package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
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
	pushUpdateFoliageFunctionName = "function.adapter.arch_model.push_update"
	postProcessFnName             = "function.adapter.arch_model.post_process"
	archModelRootUUID             = "arch_model"
)

var (
	// natsURL - nats server url
	natsURL string = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
)

func deleteArchModel(dbc db.DBSyncClient, modelUUID string) {
	if uuids, err := dbc.Query.JPGQLCtraQuery(modelUUID, fmt.Sprintf(".*[l:type('%s')]", types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK)); err == nil {
		for _, blockUUID := range uuids {
			if err := dbc.CMDB.ObjectDelete(blockUUID); err != nil {
				lg.Logln(lg.ErrorLevel, "cannot block with uuid=%s: %v", blockUUID, err)
			}
		}
	}
	if err := dbc.CMDB.ObjectDelete(modelUUID); err != nil {
		lg.Logln(lg.ErrorLevel, "cannot delete model with uuid=%s: %v", modelUUID, err)
	}
}

func buildArchModel(ctx *sfPlugins.StatefunContextProcessor, doc easyjson.JSON) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		return fmt.Errorf("cannot create db sync client: %v", err)
	}
	modelName := doc.GetByPath("name").AsStringDefault("unknown model")
	modelUUID := archModelRootUUID

	deleteArchModel(dbc, modelUUID)

	modelDetails := doc.GetByPath("details").Clone()
	modelDetails.SetByPath("name", easyjson.NewJSON(modelName))
	if err := dbc.CMDB.ObjectUpdate(
		modelUUID,
		modelDetails,
		false,
		types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL,
	); err != nil {
		return fmt.Errorf("ObjectUpdate failed for model %q: %w", modelName, err)
	}

	notifierPayload := easyjson.NewJSONObject()
	notifierPayload.SetByPath("domain", easyjson.NewJSON(ctx.Domain.Name()))
	notifierPayload.SetByPath("id", easyjson.NewJSON(modelUUID))
	notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL))
	notifierPayload.SetByPath("operation", easyjson.NewJSON("link_model"))
	common.PostProcessNotifier(dbc, ctx, notifierPayload)

	blocksJ := doc.GetByPath("blocks")
	blocksArr, ok := blocksJ.AsArray()
	if !ok {
		return fmt.Errorf("invalid schema: 'blocks' must be an array")
	}

	// First pass: create/update all nodes so links won't point to non-existing nodes.
	for i, raw := range blocksArr {
		block := easyjson.NewJSON(raw)

		name, ok := block.GetByPath("name").AsString()
		if !ok || name == "" {
			return fmt.Errorf("invalid schema: blocks[%d].name must be a non-empty string", i)
		}

		details := block.GetByPath("details").Clone()
		details.SetByPath("name", easyjson.NewJSON(name))
		if details.IsNull() {
			// If details are missing/null, store an empty object.
			details = easyjson.NewJSONObject()
		}
		if !details.IsObject() {
			return fmt.Errorf("invalid schema: blocks[%d].details must be an object", i)
		}

		// Use a stable ID derived from block name.
		id := system.GetHashStr(name + types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK)

		// Store details as-is (it is already easyjson.JSON).
		if err := dbc.CMDB.ObjectUpdate(
			id,
			details,
			true,
			types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK,
		); err != nil {
			return fmt.Errorf("ObjectUpdate failed for block %q: %w", name, err)
		}

		// Get service name for matching with K8s objects
		serviceName := details.GetByPath("service").AsStringDefault("")
		if serviceName == "" {
			serviceName = strings.ToLower(name)
		}

		notifierPayload := easyjson.NewJSONObject()
		notifierPayload.SetByPath("domain", easyjson.NewJSON(ctx.Domain.Name()))
		notifierPayload.SetByPath("id", easyjson.NewJSON(id))
		notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
		notifierPayload.SetByPath("operation", easyjson.NewJSON("link_arch_block"))
		notifierPayload.SetByPath("service", easyjson.NewJSON(serviceName))
		common.PostProcessNotifier(dbc, ctx, notifierPayload)

		if err := dbc.CMDB.ObjectsLinkUpdate(
			modelUUID,
			id,
			nil,
			easyjson.NewJSONObject(),
			false,
			name, // As requested: pass downstream_block_name as the last parameter.
		); err != nil {
			return fmt.Errorf("Failed to link model %s to block %s: %v", modelName, name, err)
		}
	}

	// Second pass: create/update all links.
	for i, raw := range blocksArr {
		block := easyjson.NewJSON(raw)

		fromName, ok := block.GetByPath("name").AsString()
		if !ok || fromName == "" {
			return fmt.Errorf("invalid schema: blocks[%d].name must be a non-empty string", i)
		}

		downstream := block.GetByPath("downstream")
		if downstream.IsNull() {
			// No downstream section -> no links.
			continue
		}

		downArr, ok := downstream.AsArray()
		if !ok {
			return fmt.Errorf("invalid schema: blocks[%d].downstream must be an array", i)
		}

		fromID := system.GetHashStr(fromName + types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK)

		for j, edgeRaw := range downArr {
			edge := easyjson.NewJSON(edgeRaw)

			toName, ok := edge.GetByPath("to_block_name").AsString()
			if !ok || toName == "" {
				return fmt.Errorf("invalid schema: blocks[%d].downstream[%d].to_block_name must be a non-empty string", i, j)
			}
			linkBody := edge.GetByPath("details")
			if edge.GetByPath("details").IsNonEmptyObject() {
				linkBody = edge.GetByPath("details")
			}

			toID := system.GetHashStr(toName + types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK)

			// The user requested to use an empty JSON object for the link details.
			// If you want to persist edge details (like "gRPC call"), you can extend this later.
			if err := dbc.CMDB.ObjectsLinkUpdate(
				fromID,
				toID,
				nil,
				linkBody,
				false,
				toName, // As requested: pass downstream_block_name as the last parameter.
			); err != nil {
				return fmt.Errorf("ObjectsLinkUpdate failed: %s -> %s: %v", fromName, toName, err)
			}
		}
	}

	return nil
}

func pushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "inspect cannot create db client")
		return
	}

	// Processing push update call from cn_json_file ----------------
	if strings.Contains(strings.ToLower(ctx.Caller.ID), strings.ToLower(apps.APP_CN_JSON_FILE)) {
		if uuids, err := dbc.Query.JPGQLCtraQuery(ctx.Caller.ID, fmt.Sprintf(".*[l:type('%s')]", types.TYPE_FOLIAGE_CONNECTOR_JSON_FILE)); err == nil {
			if len(uuids) > 0 {
				if data, err := dbc.CMDB.ObjectRead(uuids[0]); err == nil {
					if err = buildArchModel(ctx, data.GetByPath("body")); err != nil {
						lg.Logln(lg.ErrorLevel, "cannot build architecture model: %v", err)
						return
					}
				}
			} else {
				lg.Logln(lg.WarnLevel, "connector %s has no valid json file data", apps.APP_CN_JSON_FILE)
			}
		}
	} else {
		lg.Logln(lg.DebugLevel, "recevied push update signal, but ignored – waiting for %s but called by %s", apps.APP_CN_JSON_FILE, ctx.Caller.ID)
	}
	// --------------------------------------------------------------

	adapterUpdateStatus(dbc)
}

func adapterUpdateStatus(dbc db.DBSyncClient) {
	t := time.Now()

	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_ARCH_MODEL, data, false, types.TYPE_FOLIAGE_APP_ADAPTER))
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(runtime, pushUpdateFoliageFunctionName, pushUpdate, *statefun.NewFunctionTypeConfig())
	statefun.NewFunctionType(runtime, postProcessFnName, archModelPostProcess, *statefun.NewFunctionTypeConfig())
}

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, easyjson.NewJSONObject(), false, true))
	adapterBody := easyjson.NewJSONObject()
	adapterBody.SetByPath("push_update_function", easyjson.NewJSON(pushUpdateFoliageFunctionName))
	adapterBody.SetByPath("post_process_function", easyjson.NewJSON(postProcessFnName))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_AD_ARCH_MODEL, adapterBody, false, types.TYPE_FOLIAGE_APP_ADAPTER))

	// Init model data ----------------------------------------------
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
	// ---------------------------------------------------------------

	// Init K8s types for shadow objects from M2 --------------------
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_POD, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_DEPLOYMENT, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, types.TYPE_FOLIAGE_POD, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_POD))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, types.TYPE_FOLIAGE_DEPLOYMENT, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_DEPLOYMENT))
	// ---------------------------------------------------------------

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))

	runtime.Domain.SetWeakClusterDomains([]string{"m1", "m2", "m4"})

	go common.HeartBeat(ctx, runtime, archModelRootUUID, types.TYPE_FOLIAGE_ADAPTER_ARCH_MODEL)
	go shadowLinksKeeper(ctx, dbc, runtime)

	return nil
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_AD_ARCH_MODEL).SetDomainRoutersHandling(false).UseJSDomainAsHubDomainName()); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig(apps.APP_AD_ARCH_MODEL+"cache")); err != nil {
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
