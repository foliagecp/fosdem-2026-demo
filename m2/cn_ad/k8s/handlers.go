package main

import (
	"context"
	"fmt"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	lg "github.com/foliagecp/sdk/statefun/logger"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

// TODO type k8s, uid on link

func (w *Watcher) handleNode(ctx context.Context, eventType EventType, node *corev1.Node) {
	nodeID := string(node.UID)

	switch eventType {
	case ADD, UPDATE:
		nodeBytes, err := json.Marshal(node)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleNode: marshal error: %v", err)
			return
		}

		nodeJSON, ok := easyjson.JSONFromBytes(nodeBytes)
		if !ok {
			lg.GetLogger().Errorf(ctx, "handleNode: invalid json")
			return
		}

		body := easyjson.NewJSONObjectWithKeyValue("raw", nodeJSON)

		err = w.dbc.ObjectUpdate(nodeID, body, true, m2.K8S_INFORMER_TYPE)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleNode: ObjectUpdate error: %v", err)
			return
		}

		err = w.dbc.ObjectsLinkUpdate(
			runtimeName,
			nodeID,
			[]string{fmt.Sprintf(m2.TAG_SOURCE_TYPE_NODE, "node")},
			easyjson.NewJSONObject(),
			true,
			nodeID,
		)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleNode: link error: %v", err)
			return
		}

		lg.GetLogger().Infof(ctx, "Node %s (%s) synced", node.Name, nodeID)

	case DELETE:
		err := w.dbc.ObjectDelete(nodeID)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleNode: delete error: %v", err)
		}
		lg.GetLogger().Infof(ctx, "Node %s (%s) deleted", node.Name, nodeID)
	}
}

func (w *Watcher) handlePod(ctx context.Context, eventType EventType, pod *corev1.Pod) {
	podID := string(pod.UID)

	switch eventType {
	case ADD, UPDATE:
		podBytes, err := json.Marshal(pod)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handlePod: marshal error: %v", err)
			return
		}

		podJSON, ok := easyjson.JSONFromBytes(podBytes)
		if !ok {
			lg.GetLogger().Errorf(ctx, "handlePod: invalid json")
			return
		}

		body := easyjson.NewJSONObjectWithKeyValue("raw", podJSON)

		err = w.dbc.ObjectUpdate(podID, body, true, m2.K8S_INFORMER_TYPE)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handlePod: ObjectUpdate error: %v", err)
			return
		}

		err = w.dbc.ObjectsLinkUpdate(
			runtimeName,
			podID,
			[]string{fmt.Sprintf(m2.TAG_SOURCE_TYPE_POD, "pod")},
			easyjson.NewJSONObject(),
			true,
			podID,
		)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handlePod: link error: %v", err)
			return
		}

		lg.GetLogger().Infof(ctx, "Pod %s (%s) synced", pod.Name, podID)

	case DELETE:
		err := w.dbc.ObjectDelete(podID)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handlePod: ObjectDelete error: %v", err)
			return
		}

		lg.GetLogger().Infof(ctx, "Pod %s (%s) deleted", pod.Name, podID)
	}
}

func (w *Watcher) handleDeployment(ctx context.Context, eventType EventType, deployment *appsv1.Deployment) {
	deploymentID := string(deployment.UID)

	switch eventType {
	case ADD, UPDATE:
		deploymentBytes, err := json.Marshal(deployment)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleDeployment: marshal error: %v", err)
			return
		}

		deploymentJSON, ok := easyjson.JSONFromBytes(deploymentBytes)
		if !ok {
			lg.GetLogger().Errorf(ctx, "handleDeployment: invalid json")
			return
		}

		body := easyjson.NewJSONObjectWithKeyValue("raw", deploymentJSON)

		err = w.dbc.ObjectUpdate(deploymentID, body, true, m2.K8S_INFORMER_TYPE)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleDeployment: ObjectUpdate error: %v", err)
			return
		}

		err = w.dbc.ObjectsLinkUpdate(
			runtimeName,
			deploymentID,
			[]string{fmt.Sprintf(m2.TAG_SOURCE_TYPE_DEPLOYMENT, "deployment")},
			easyjson.NewJSONObject(),
			true,
			deploymentID,
		)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleDeployment: link error: %v", err)
			return
		}

		lg.GetLogger().Infof(ctx, "Deployment %s/%s (%s) synced", deployment.Namespace, deployment.Name, deploymentID)

	case DELETE:
		err := w.dbc.ObjectDelete(deploymentID)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleDeployment: delete error: %v", err)
		}
		lg.GetLogger().Infof(ctx, "Deployment %s/%s (%s) deleted", deployment.Namespace, deployment.Name, deploymentID)
	}
}

func (w *Watcher) handleReplicaSet(ctx context.Context, eventType EventType, rs *appsv1.ReplicaSet) {
	rsID := string(rs.UID)

	switch eventType {
	case ADD, UPDATE:
		rsBytes, err := json.Marshal(rs)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleReplicaSet: marshal error: %v", err)
			return
		}

		rsJSON, ok := easyjson.JSONFromBytes(rsBytes)
		if !ok {
			lg.GetLogger().Errorf(ctx, "handleReplicaSet: invalid json")
			return
		}

		body := easyjson.NewJSONObjectWithKeyValue("raw", rsJSON)

		err = w.dbc.ObjectUpdate(rsID, body, true, m2.K8S_INFORMER_TYPE)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleReplicaSet: ObjectUpdate error: %v", err)
			return
		}

		err = w.dbc.ObjectsLinkUpdate(
			runtimeName,
			rsID,
			[]string{fmt.Sprintf(m2.TAG_SOURCE_TYPE_REPLICATION_SET, "replicaset")},
			easyjson.NewJSONObject(),
			true,
			rsID,
		)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleReplicaSet: link error: %v", err)
			return
		}

		lg.GetLogger().Infof(ctx, "ReplicaSet %s/%s (%s) synced", rs.Namespace, rs.Name, rsID)

	case DELETE:
		err := w.dbc.ObjectDelete(rsID)
		if err != nil {
			lg.GetLogger().Errorf(ctx, "handleReplicaSet: delete error: %v", err)
		}
		lg.GetLogger().Infof(ctx, "ReplicaSet %s/%s (%s) deleted", rs.Namespace, rs.Name, rsID)
	}
}
