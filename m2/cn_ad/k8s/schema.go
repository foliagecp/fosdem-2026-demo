package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	"github.com/foliagecp/sdk/statefun/system"
	"k8s.io/client-go/tools/clientcmd"
)

func createSchema(dbc *db.DBSyncClient) {
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.CONNECTOR_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.CLUSTER_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.NODE_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.POD_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.DEPLOYMENT_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.REPLICATION_SET_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.K8S_INFORMER_TYPE, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkCreate(m2.CONNECTOR_TYPE, m2.CLUSTER_TYPE, m2.CLUSTER_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkCreate(m2.CONNECTOR_TYPE, m2.K8S_INFORMER_TYPE, m2.K8S_INFORMER_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkCreate(m2.CONNECTOR_TYPE, m2.NODE_TYPE, m2.NODE_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkCreate(m2.CONNECTOR_TYPE, m2.POD_TYPE, m2.POD_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkCreate(m2.CONNECTOR_TYPE, m2.DEPLOYMENT_TYPE, m2.DEPLOYMENT_TYPE, nil))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkCreate(m2.CONNECTOR_TYPE, m2.REPLICATION_SET_TYPE, m2.REPLICATION_SET_TYPE, nil))

	system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(runtimeName, m2.CONNECTOR_TYPE))

	clusterBody, err := bodyFromKubeconfigFile(kubeconfigPath)
	if err != nil {
		lg.GetLogger().Errorf(context.TODO(), "error read kubeconfig: %v", err)
		return
	}
	//TODO name for root object?
	system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate("cluster", m2.K8S_INFORMER_TYPE, clusterBody))
	system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkCreate(runtimeName, "cluster", "cluster", []string{m2.TAG_SOURCE_TYPE_CLUSTER}))

	config(kubeconfigPath)
}

func bodyFromKubeconfigFile(filePath string) (easyjson.JSON, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return easyjson.NewJSONObject(), err
	}

	return easyjson.NewJSON(string(data)), nil
}

func config(kubeconfigPath string) {
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		log.Fatalf("Ошибка загрузки: %v", err)
	}

	fmt.Println("Текущий контекст:", config.CurrentContext)

	for name, cluster := range config.Clusters {
		fmt.Printf("Кластер: %s, Сервер: %s\n", name, cluster.Server)
	}

	ctx := config.Contexts[config.CurrentContext]
	if user, ok := config.AuthInfos[ctx.AuthInfo]; ok {
		fmt.Printf("Пользователь: %s, Токен: %s\n", ctx.AuthInfo, user.Token)
	}
}
