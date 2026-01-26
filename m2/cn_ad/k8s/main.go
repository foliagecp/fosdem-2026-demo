package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	"github.com/foliagecp/sdk/statefun/cache"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
	k8s "k8s.io/client-go/kubernetes"
)

const (
	pushUpdateFnName          = "function.cn_ad.k8s.push_update"
	postProcessFnName         = "function.cn_ad.k8s.post_process"
	propagateErrorFnName      = "function.cn_ad.k8s.propagate_error"
	k8sInfrastructureRootUUID = "k8s_infrastructure"
)

var (
	natsURL        = system.GetEnvMustProceed("NATS_URL", "nats://nats:foliage@nats:4222")
	kubeconfigPath = system.GetEnvMustProceed("KUBECONFIG_PATH", "/kubeconfig")
	dumpMode       = system.GetEnvMustProceed("DUMP_MODE", true)
	dumpFile       = system.GetEnvMustProceed("DUMP_FILE", "/dumps/kube_dump.yaml")
)

func onAfterStart(ctx context.Context, runtime *statefun.Runtime) error {
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return err
	}

	var (
		k8sClient   k8s.Interface
		clusterID   string
		clusterName string
	)

	if dumpMode {
		lg.GetLogger().Infof(ctx, "dump mode, dump file: %s", dumpFile)
		k8sClient, err = NewFakeK8sClientFromDump(dumpFile)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "create k8s client from dump error: %v", err)
			return err
		}
		clusterID = "fake_k8s_cluster_id"
		clusterName = "fake_k8s_cluster_name"
	} else {
		k8sClient, err = NewK8sClient(kubeconfigPath)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "create k8s client error: %v", err)
			return err
		}
		clusterName = GetClusterNameFromConfig(kubeconfigPath)
		clusterID, err = GetClusterIDFromK8sClient(k8sClient)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "get cluster id from k8s client error: %v", err)
			return err
		}
	}

	createScheme(dbc)

	adapterBody := easyjson.NewJSONObject()
	adapterBody.SetByPath("push_update_function", easyjson.NewJSON(pushUpdateFnName))
	adapterBody.SetByPath("post_process_function", easyjson.NewJSON(postProcessFnName))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(apps.APP_CN_AD_K8S, adapterBody, true, types.TYPE_FOLIAGE_APP_ADAPTER))

	k8sInfrastructureObjectID := k8sInfrastructureRootUUID
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(k8sInfrastructureObjectID, easyjson.NewJSONObject(), true, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(apps.APP_CN_AD_K8S, k8sInfrastructureObjectID, nil, easyjson.NewJSONObject(), true, k8sInfrastructureObjectID))

	clusterBody := easyjson.NewJSONObject()
	clusterBody.SetByPath("cluster_name", easyjson.NewJSON(clusterName))
	clusterBody.SetByPath("cluster_id", easyjson.NewJSON(clusterID))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(clusterID, clusterBody, true, types.TYPE_FOLIAGE_CLUSTER))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(k8sInfrastructureObjectID, clusterID, nil, easyjson.NewJSONObject(), true, clusterID))

	stopCh := make(chan struct{})

	_, err = NewWatcher(runtime, k8sClient, k8sInfrastructureObjectID, clusterID, stopCh)
	if err != nil {
		lg.GetLogger().Errorf(ctx, "create k8s watcher error: %v", err)
		return err
	}

	runtime.Domain.SetWeakClusterDomains([]string{"m1", "m3", "m4"})

	go func() {
		<-ctx.Done()
		close(stopCh)
	}()

	go common.HeartBeat(ctx, runtime, k8sInfrastructureRootUUID, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE)
	go shadowLinksKeeper(ctx, dbc, runtime)

	return nil
}

func pushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	system.MsgOnErrorReturn(ctx.Signal(
		sfPlugins.AutoSignalSelect,
		"functions.cn_ad.k8s.build",
		ctx.Self.ID,
		nil,
		nil,
	))
}

func registerFunctionTypes(runtime *statefun.Runtime) {
	statefun.NewFunctionType(
		runtime,
		"functions.cn_ad.k8s.build",
		build,
		*statefun.NewFunctionTypeConfig().SetAllowedSignalProviders(sfPlugins.AutoSignalSelect),
	)
	statefun.NewFunctionType(
		runtime,
		"functions.cn_ad.k8s.status",
		status,
		*statefun.NewFunctionTypeConfig().SetAllowedRequestProviders(sfPlugins.AutoRequestSelect),
	)
	statefun.NewFunctionType(
		runtime,
		pushUpdateFnName,
		pushUpdate,
		*statefun.NewFunctionTypeConfig().SetAllowedSignalProviders(sfPlugins.AutoSignalSelect),
	)
	statefun.NewFunctionType(
		runtime,
		postProcessFnName,
		postProcess,
		*statefun.NewFunctionTypeConfig().SetAllowedSignalProviders(sfPlugins.AutoSignalSelect),
	)
	statefun.NewFunctionType(
		runtime,
		propagateErrorFnName,
		common.PropagateError,
		*statefun.NewFunctionTypeConfig().SetAllowedSignalProviders(sfPlugins.AutoSignalSelect),
	)
}

func adapterUpdateStatus(dbc db.DBSyncClient) {
	t := time.Now()

	data := easyjson.NewJSONObject()
	data.SetByPath("updated_at.datetime", easyjson.NewJSON(t.Format("2006-01-02 15:04:05 MST")))
	data.SetByPath("updated_at.nano", easyjson.NewJSON(t.UnixNano()))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(k8sInfrastructureRootUUID, data, false, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE))
}

func start() {
	system.GlobalPrometrics = system.NewPrometrics("", ":9901")
	if runtime, err := statefun.NewRuntime(*statefun.NewRuntimeConfigSimple(natsURL, apps.APP_CN_AD_K8S).
		SetDomainRoutersHandling(false).UseJSDomainAsHubDomainName()); err == nil {
		registerFunctionTypes(runtime)
		runtime.RegisterOnAfterStartFunction(onAfterStart, false)
		if err = runtime.Start(context.TODO(), cache.NewCacheConfig("cn_ad_cache")); err != nil {
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
