package main

import (
	"fmt"

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

type EventType = string

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

func NewWatcher(runtime *statefun.Runtime, k8sClient *k8s.Clientset, clusterID string, stopCh <-chan struct{}) (*Watcher, error) {
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
			m2Object.SetByPath("ownerKind", easyjson.NewJSON(ownerReferences[0].Kind))
			m2Object.SetByPath("ownerUID", easyjson.NewJSON(string(ownerReferences[0].UID)))
			m2Object.SetByPath("ownerName", easyjson.NewJSON(ownerReferences[0].Name))
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
			m2Object.SetByPath("ownerKind", easyjson.NewJSON(ownerReferences[0].Kind))
			m2Object.SetByPath("ownerUID", easyjson.NewJSON(string(ownerReferences[0].UID)))
			m2Object.SetByPath("ownerName", easyjson.NewJSON(ownerReferences[0].Name))
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
		system.MsgOnErrorReturn(w.dbc.ObjectDelete(objID))
		//TODO kick adapter
	case ADD, UPDATE:
		if err := w.dbc.ObjectUpdate(objID, body, true, typeName); err != nil {
			system.MsgOnErrorReturn(err)
			return
		}
		system.MsgOnErrorReturn(w.dbc.ObjectsLinkUpdate(w.clusterID, objID, []string{typeName}, easyjson.NewJSONObject(), true, objID))
		//TODO kick adapter
	}
}

func GetClusterIDFromK8sClient(k8sClient *k8s.Clientset) (string, error) {
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
