package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	"github.com/foliagecp/fosdem-2026-demo/m3/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
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

const (
	ADD    EventType = "Add"
	UPDATE EventType = "Update"
	DELETE EventType = "Delete"
)

const (
	k8sSystemNamespaceKubeSystem    = "kube-system"
	k8sSystemNamespaceKubePublic    = "kube-public"
	k8sSystemNamespaceKubeNodeLease = "kube-node-lease"
)

const (
	deploymentKind  = "Deployment"
	replicaSetKind  = "ReplicaSet"
	nodeKind        = "Node"
	statefulSetKind = "StatefulSet"
	daemonSetKind   = "DaemonSet"
	jobKind         = "Job"
	cronJobKind     = "CronJob"
)

type EventType = string

type Watcher struct {
	runtime          *statefun.Runtime
	dbc              *db.DBSyncClient
	clusterID        string
	nodeLister       core.NodeLister
	podLister        core.PodLister
	deploymentLister apps.DeploymentLister
	replicaSetLister apps.ReplicaSetLister

	// rebuild
	rebuildMu         sync.Mutex
	delayRebuildTimer *time.Timer
	rebuildPending    bool
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

func NewWatcher(runtime *statefun.Runtime, k8sClient k8s.Interface, clusterID string, stopCh <-chan struct{}) (*Watcher, error) {
	factory := informers.NewSharedInformerFactory(k8sClient, 0)

	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		runtime:           runtime,
		dbc:               &dbc,
		clusterID:         clusterID,
		nodeLister:        factory.Core().V1().Nodes().Lister(),
		podLister:         factory.Core().V1().Pods().Lister(),
		deploymentLister:  factory.Apps().V1().Deployments().Lister(),
		replicaSetLister:  factory.Apps().V1().ReplicaSets().Lister(),
		rebuildMu:         sync.Mutex{},
		delayRebuildTimer: nil,
		rebuildPending:    false,
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
	var (
		typeName string
		objectID string
	)
	m2Object := easyjson.NewJSONObject()

	switch resource := obj.(type) {
	case *corev1.Node:
		objectID = string(resource.UID)

		m2Object.SetByPath("UID", easyjson.NewJSON(objectID))
		m2Object.SetByPath("name", easyjson.NewJSON(resource.Name))
		m2Object.SetByPath("systemUID", easyjson.NewJSON(resource.Status.NodeInfo.SystemUUID))

		typeName = m2.NODE_TYPE

	case *corev1.Pod:
		objectID = string(resource.UID)

		m2Object.SetByPath("UID", easyjson.NewJSON(objectID))
		m2Object.SetByPath("name", easyjson.NewJSON(resource.Name))
		m2Object.SetByPath("statusPhase", easyjson.NewJSON(string(resource.Status.Phase)))
		m2Object.SetByPath("creationTimestamp", easyjson.NewJSON(resource.CreationTimestamp.String()))
		m2Object.SetByPath("nodeName", easyjson.NewJSON(resource.Spec.NodeName))
		m2Object.SetByPath("labelsApp", easyjson.NewJSON(resource.Labels["app"]))
		if containers := resource.Spec.Containers; len(containers) > 0 {
			m2Object.SetByPath("containersImage", easyjson.NewJSON(containers[0].Image))
		}
		if ownerReferences := resource.OwnerReferences; len(ownerReferences) > 0 {
			for _, ownerReference := range ownerReferences {
				if ownerReference.Controller != nil && *ownerReference.Controller {
					m2Object.SetByPath("ownerKind", easyjson.NewJSON(ownerReferences[0].Kind))
					m2Object.SetByPath("ownerUID", easyjson.NewJSON(string(ownerReferences[0].UID)))
					m2Object.SetByPath("ownerName", easyjson.NewJSON(ownerReferences[0].Name))
				}
			}
		}

		typeName = m2.POD_TYPE

	case *appsv1.Deployment:
		objectID = string(resource.UID)

		m2Object.SetByPath("UID", easyjson.NewJSON(objectID))
		m2Object.SetByPath("name", easyjson.NewJSON(resource.Name))
		if containers := resource.Spec.Template.Spec.Containers; len(containers) > 0 {
			m2Object.SetByPath("containersImage", easyjson.NewJSON(containers[0].Image))
		}
		typeName = m2.DEPLOYMENT_TYPE

	case *appsv1.ReplicaSet:
		objectID = string(resource.UID)

		m2Object.SetByPath("UID", easyjson.NewJSON(objectID))
		m2Object.SetByPath("name", easyjson.NewJSON(resource.Name))
		if ownerReferences := resource.OwnerReferences; len(ownerReferences) > 0 {
			for _, ownerReference := range ownerReferences {
				if ownerReference.Controller != nil && *ownerReference.Controller {
					m2Object.SetByPath("ownerKind", easyjson.NewJSON(ownerReferences[0].Kind))
					m2Object.SetByPath("ownerUID", easyjson.NewJSON(string(ownerReferences[0].UID)))
					m2Object.SetByPath("ownerName", easyjson.NewJSON(ownerReferences[0].Name))
				}
			}
		}

		typeName = m2.REPLICATION_SET_TYPE

	default:
		lg.GetLogger().Warnf(context.TODO(), "No metadata mapping for type: %T", obj)
		return
	}

	w.processResource(eventType, objectID, m2Object, typeName)
}

func (w *Watcher) processResource(eventType EventType, objID string, body easyjson.JSON, typeName string) {
	switch eventType {
	case DELETE:
		system.MsgOnErrorReturn(w.dbc.CMDB.ObjectDelete(objID))
		if typeName == m2.POD_TYPE || typeName == m2.DEPLOYMENT_TYPE {
			body.SetByPath("type", easyjson.NewJSON(typeName))
			body.SetByPath("operation", easyjson.NewJSON("delete"))
			w.notifyAdapters(&body)
		}
	case ADD:
		if err := w.dbc.CMDB.ObjectCreate(objID, typeName, body); err != nil {
			system.MsgOnErrorReturn(err)
			return
		}
		system.MsgOnErrorReturn(w.dbc.CMDB.ObjectsLinkUpdate(w.clusterID, objID, []string{typeName}, easyjson.NewJSONObject(), false, objID))
		if typeName == m2.POD_TYPE || typeName == m2.DEPLOYMENT_TYPE {
			body.SetByPath("type", easyjson.NewJSON(typeName))
			body.SetByPath("operation", easyjson.NewJSON("add"))
			w.notifyAdapters(&body)
		}
	case UPDATE:
		if err := w.dbc.CMDB.ObjectUpdate(objID, body, false, typeName); err != nil {
			system.MsgOnErrorReturn(err)
			return
		}
		system.MsgOnErrorReturn(w.dbc.CMDB.ObjectsLinkUpdate(w.clusterID, objID, []string{typeName}, easyjson.NewJSONObject(), false, objID))
	}

	w.markDirty()
}

func (w *Watcher) markDirty() {
	w.rebuildMu.Lock()
	defer w.rebuildMu.Unlock()

	w.rebuildPending = true

	if w.delayRebuildTimer != nil {
		w.delayRebuildTimer.Reset(300 * time.Millisecond)
		return
	}

	w.delayRebuildTimer = time.AfterFunc(300*time.Millisecond, w.rebuild)
}

func (w *Watcher) rebuild() {
	w.rebuildMu.Lock()
	if !w.rebuildPending {
		w.rebuildMu.Unlock()
		return
	}

	w.rebuildPending = false
	w.delayRebuildTimer = nil
	w.rebuildMu.Unlock()

	system.MsgOnErrorReturn(
		w.runtime.Signal(
			sfPlugins.AutoSignalSelect,
			"functions.cn_ad.k8s.build",
			w.clusterID,
			nil,
			nil,
		),
	)
}

func (w *Watcher) notifyAdapters(body *easyjson.JSON) {
	getPushUpdateFunction := func(adapterUUID string) (string, bool) {
		if data, err := w.dbc.CMDB.ObjectRead(adapterUUID); err == nil {
			return data.GetByPath("body.push_update_function").AsString()
		}
		return "", false
	}
	for _, dm := range w.runtime.Domain.GetWeakClusterDomains() {
		if uuids, err := w.dbc.Query.JPGQLCtraQuery(w.runtime.Domain.CreateObjectIDWithDomain(dm, types.TYPE_FOLIAGE_APP_ADAPTER, true), ".*[l:type('__object')]"); err == nil {
			for _, uuid := range uuids {
				if typename, ok := getPushUpdateFunction(uuid); ok {
					system.MsgOnErrorReturn(w.runtime.Signal(sfPlugins.AutoSignalSelect, typename, uuid, body, nil))
				}
			}
		}
	}
}

func GetClusterIDFromK8sClient(k8sClient k8s.Interface) (string, error) {
	ctx := context.Background()
	ns, err := k8sClient.CoreV1().Namespaces().Get(ctx, k8sSystemNamespaceKubeSystem, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(ns.UID), nil
}

func GetNamespaceFromConfig(kubeconfigPath string) (string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

	ns, _, err := clientConfig.Namespace()
	if err != nil {
		return "", fmt.Errorf("GetNamespaceFromConfig: %v", err)
	}
	return ns, nil
}

func GetClusterNameFromConfig(kubeconfigPath string) string {
	const defaultClusterName = "default_cluster_name"
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		lg.GetLogger().Errorf(context.TODO(), "GetClusterNameFromConfig: %v", err)
		return defaultClusterName
	}

	contextObj, ok := config.Contexts[config.CurrentContext]
	if !ok {
		lg.GetLogger().Errorf(context.TODO(), "GetClusterNameFromConfig: context not found")
		return defaultClusterName
	}

	return contextObj.Cluster
}
