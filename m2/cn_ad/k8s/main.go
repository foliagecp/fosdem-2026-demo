package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfMediators "github.com/foliagecp/sdk/statefun/mediator"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

const (
	runtimeName = "k8s_cn_ad"
)

var (
	natsURL        = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
	kubeconfigPath = system.GetEnvMustProceed("KUBECONFIG_PATH", "m2/configs/kubeconfig")
	lastUpdateTime atomic.Int64
	running        atomic.Int32
)

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	k8sClient, err := NewK8sClient(kubeconfigPath)
	if err != nil {
		lg.GetLogger().Errorf(ctx, "create k8s client error: %v", err)
		return err
	}

	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.CONNECTOR_ADAPTER_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.CLUSTER_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.NODE_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.POD_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.DEPLOYMENT_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.REPLICATION_SET_TYPE, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CONNECTOR_ADAPTER_TYPE, m2.CLUSTER_TYPE, nil, easyjson.NewJSONObject(), false, m2.CLUSTER_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.NODE_TYPE, m2.NODE_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.POD_TYPE, m2.POD_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.DEPLOYMENT_TYPE, m2.DEPLOYMENT_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.REPLICATION_SET_TYPE, m2.REPLICATION_SET_TYPE, nil))

	clusterName := GetClusterNameFromConfig(kubeconfigPath)
	clusterID, err := GetClusterIDFromK8sClient(k8sClient)
	if err != nil {
		lg.GetLogger().Errorf(ctx, "get cluster id from k8s client error: %v", err)
		return err
	}
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(clusterID, easyjson.NewJSONObject(), true, m2.CONNECTOR_ADAPTER_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(clusterID, easyjson.NewJSONObject(), true, m2.CLUSTER_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(clusterName, clusterID, nil, easyjson.NewJSONObject(), true, clusterName))

	stopCh := make(chan struct{})

	_, err = NewWatcher(runtime, k8sClient, clusterID, stopCh)
	if err != nil {
		lg.GetLogger().Errorf(ctx, "create k8s watcher error: %v", err)
		return err
	}

	go func() {
		<-ctx.Done()
		close(stopCh)
	}()

	return nil
}

func status(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	om := sfMediators.NewOpMediator(ctx)

	data := easyjson.NewJSONObjectWithKeyValue("running", easyjson.NewJSON(running.Load() > 0))
	data.SetByPath("last_run_finish_time", easyjson.NewJSON(lastUpdateTime.Load()))

	om.AggregateOpMsg(sfMediators.OpMsgOk(data)).Reply()
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	//statefun.NewFunctionType(runtime, "functions.connectors.k8s_api.inspect", inspect, *statefun.NewFunctionTypeConfig())
	//statefun.NewFunctionType(runtime, "functions.connectors.k8s_api.add", inspect, *statefun.NewFunctionTypeConfig())
	//statefun.NewFunctionType(runtime, "functions.connectors.k8s_api.update", inspect, *statefun.NewFunctionTypeConfig())
	//statefun.NewFunctionType(runtime, "functions.connectors.k8s_api.delete", inspect, *statefun.NewFunctionTypeConfig())
	statefun.NewFunctionType(runtime, "functions.connectors.k8s_api.status", status, *statefun.NewFunctionTypeConfig().SetAllowedRequestProviders(sfPlugins.AutoRequestSelect))
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, runtimeName).UseJSDomainAsHubDomainName()); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err := runtime.Start(context.TODO(), cache.NewCacheConfig("cn_ad_cache")); err != nil {
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
