package main

import (
	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	"github.com/foliagecp/sdk/statefun/system"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	apps "k8s.io/client-go/listers/apps/v1"
	core "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type EventType = string

const (
	ADD    EventType = "Add"
	UPDATE EventType = "Update"
	DELETE EventType = "Delete"
)

const k8sSystemNamespace = "kube-system"

type Watcher struct {
	runtime          *statefun.Runtime
	dbc              *db.CMDBSyncClient
	clusterID        string
	nodeLister       core.NodeLister
	podLister        core.PodLister
	deploymentLister apps.DeploymentLister
	replicaSetLister apps.ReplicaSetLister
}

func NewK8sClient(kubeconfigPath string) (*k8s.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	clientSet, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientSet, nil
}

func NewWatcher(runtime *statefun.Runtime, k8sClient *k8s.Clientset, stopCh <-chan struct{}, clusterID string) (*Watcher, error) {
	factory := informers.NewSharedInformerFactory(k8sClient, 0)

	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		runtime:          runtime,
		dbc:              &dbc.CMDB,
		clusterID:        clusterID,
		nodeLister:       factory.Core().V1().Nodes().Lister(),
		podLister:        factory.Core().V1().Pods().Lister(),
		deploymentLister: factory.Apps().V1().Deployments().Lister(),
		replicaSetLister: factory.Apps().V1().ReplicaSets().Lister(),
	}

	genericHandler := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.sync(ADD, obj) },
		UpdateFunc: func(old, new interface{}) { w.sync(UPDATE, new) },
		DeleteFunc: func(obj interface{}) { w.sync(DELETE, obj) },
	}

	factory.Core().V1().Nodes().Informer().AddEventHandler(genericHandler)
	factory.Core().V1().Pods().Informer().AddEventHandler(genericHandler)
	factory.Apps().V1().ReplicaSets().Informer().AddEventHandler(genericHandler)
	factory.Apps().V1().Deployments().Informer().AddEventHandler(genericHandler)

	factory.Start(stopCh)

	factory.WaitForCacheSync(stopCh)

	return w, nil
}

func (w *Watcher) sync(eventType EventType, obj interface{}) {
	ctx := context.Background()

	kubeObj, ok := obj.(metav1.Object)
	if !ok {
		lg.GetLogger().Errorf(ctx, "object is not a metav1.Object")
		return
	}

	var typeName string

	switch obj.(type) {
	case *corev1.Node:
		typeName = m2.TAG_SOURCE_TYPE_NODE
	case *corev1.Pod:
		typeName = m2.TAG_SOURCE_TYPE_POD
	case *appsv1.Deployment:
		typeName = m2.TAG_SOURCE_TYPE_DEPLOYMENT
	case *appsv1.ReplicaSet:
		typeName = m2.TAG_SOURCE_TYPE_REPLICATION_SET
	default:
		lg.GetLogger().Warnf(ctx, "No metadata mapping for type: %T", obj)
		return
	}

	w.processResource(ctx, eventType, kubeObj, obj, typeName)
}

func (w *Watcher) processResource(ctx context.Context, eventType EventType, kubeObj metav1.Object, rawObj interface{}, typeName string) {
	objID := string(kubeObj.GetUID())

	displayName := kubeObj.GetName()
	if ns := kubeObj.GetNamespace(); ns != "" {
		displayName = ns + "/" + displayName
	}

	switch eventType {
	case DELETE:
		system.MsgOnErrorReturn(w.dbc.ObjectDelete(objID))
	case ADD, UPDATE:
		body := easyjson.NewJSON(rawObj)
		if err := w.dbc.ObjectUpdate(objID, body, true, m2.K8S_INFORMER_TYPE); err != nil {
			system.MsgOnErrorReturn(err)
			return
		}
		system.MsgOnErrorReturn(w.dbc.ObjectsLinkUpdate(runtimeName, objID, []string{typeName}, easyjson.NewJSONObject(), true, objID))
	}
}

func getClusterIDFromK8s(k8sClient *k8s.Clientset) (string, error) {
	ctx := context.Background()
	ns, err := k8sClient.CoreV1().Namespaces().Get(ctx, k8sSystemNamespace, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(ns.UID), nil
}
