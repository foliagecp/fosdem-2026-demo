package main

import (
	"context"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfMediators "github.com/foliagecp/sdk/statefun/mediator"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

type Resources struct {
	CPU struct {
		Capacity    float64 `json:"capacity"`
		Allocatable float64 `json:"allocatable"`
		Requests    float64 `json:"requests"`
		Limits      float64 `json:"limits"`
	} `json:"cpu"`
	Memory struct {
		Capacity    float64 `json:"capacity"`
		Allocatable float64 `json:"allocatable"`
		Requests    float64 `json:"requests"`
		Limits      float64 `json:"limits"`
	} `json:"memory"`
}

func build(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()

	funcCtx := ctx.GetFunctionContext()
	rebuildVersion := funcCtx.GetByPath("rebuildVersion").AsNumericDefault(0)
	le.Infof(logCtx, "function 'buildLinks' has started, current version: %v", rebuildVersion)
	rebuildVersion++
	funcCtx.SetByPath("rebuildVersion", easyjson.NewJSON(rebuildVersion))
	ctx.SetFunctionContext(funcCtx)

	const foliageReadFunc = "functions.cmdb.api.object.read"

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		le.Errorf(logCtx, "buildLinks: cannot create db client: %v", err)
		return
	}
	clusters, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_CLUSTER, ".*[l:type('__object')]")
	if err != nil {
		le.Errorf(logCtx, "buildLinks: cannot query clusters: %v", err)
		return
	}
	for _, clusterID := range clusters {
		res := new(Resources)

		nodes, err := ctx.ObjectRequest(
			sfPlugins.AutoRequestSelect,
			sfPlugins.NewLinkQuery(types.TYPE_FOLIAGE_NODE),
			foliageReadFunc,
			clusterID,
			nil,
			nil,
		)
		if err != nil {
			le.Errorf(logCtx, "buildLinks: cannot query nodes: %v", err)
			return
		}

		nodesNameMap := make(map[string]string, len(nodes))
		for nodeID, node := range nodes {
			if node.ReqError == nil {
				res.CPU.Capacity += node.ReqReply.GetByPath("data.body.cpuCapacityMilli").AsNumericDefault(0)
				res.Memory.Capacity += node.ReqReply.GetByPath("data.body.memCapacityBytes").AsNumericDefault(0)
				res.CPU.Allocatable += node.ReqReply.GetByPath("data.body.cpuAllocatableMilli").AsNumericDefault(0)
				res.Memory.Allocatable += node.ReqReply.GetByPath("data.body.memAllocatableBytes").AsNumericDefault(0)

				nodeName, ok := node.ReqReply.GetByPath("data.body.name").AsString()
				if !ok {
					le.Warnf(logCtx, "buildLinks: cant get nodeName from node object: %v", nodeID)
					continue
				}
				nodesNameMap[nodeName] = nodeID
			}
		}

		pods, err := ctx.ObjectRequest(
			sfPlugins.AutoRequestSelect,
			sfPlugins.NewLinkQuery(types.TYPE_FOLIAGE_POD),
			foliageReadFunc,
			clusterID,
			nil,
			nil,
		)
		if err != nil {
			le.Errorf(logCtx, "buildLinks: cannot query pods: %v", err)
			return
		}

		deployments, err := ctx.ObjectRequest(
			sfPlugins.AutoRequestSelect,
			sfPlugins.NewLinkQuery(types.TYPE_FOLIAGE_DEPLOYMENT),
			foliageReadFunc,
			clusterID,
			nil,
			nil,
		)
		if err != nil {
			le.Errorf(logCtx, "buildLinks: cannot query deployments: %v", err)
			return
		}
		replicasets, err := ctx.ObjectRequest(
			sfPlugins.AutoRequestSelect,
			sfPlugins.NewLinkQuery(types.TYPE_FOLIAGE_REPLICATION_SET),
			foliageReadFunc,
			clusterID,
			nil,
			nil,
		)
		if err != nil {
			le.Errorf(logCtx, "buildLinks: cannot query replicasets: %v", err)
			return
		}

		for podID, pod := range pods {
			if pod.ReqError == nil {
				res.CPU.Limits += pod.ReqReply.GetByPath("data.body.cpuLimitsMilli").AsNumericDefault(0)
				res.CPU.Requests += pod.ReqReply.GetByPath("data.body.cpuRequestsMilli").AsNumericDefault(0)
				res.Memory.Limits += pod.ReqReply.GetByPath("data.body.memLimitsBytes").AsNumericDefault(0)
				res.Memory.Requests += pod.ReqReply.GetByPath("data.body.memRequestsBytes").AsNumericDefault(0)

				nodeName, ok := pod.ReqReply.GetByPath("data.body.nodeName").AsString()
				if ok {
					nodeID, ok := nodesNameMap[nodeName]
					if ok {
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(podID, nodeID, common.ErrorPropagateLinkTags, easyjson.NewJSONObject(), false, nodeID))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(nodeID, podID, common.ErrorRelyLinkTags, easyjson.NewJSONObject(), false, podID))
					}
				}
				ownerKind, ok := pod.ReqReply.GetByPath("data.body.ownerKind").AsString()
				if !ok {
					le.Warnf(logCtx, "buildLinks: cant get ownerKind from pod: %v", podID)
					continue
				}
				ownerUID, ok := pod.ReqReply.GetByPath("data.body.ownerUID").AsString()
				if !ok {
					le.Warnf(logCtx, "buildLinks: cant get ownerUID from pod: %v", podID)
					continue
				}
				switch ownerKind {
				case deploymentKind:
					if _, ok = deployments[ctx.Domain.CreateObjectIDWithDomain(ctx.Domain.Name(), ownerUID, false)]; ok {
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(podID, ownerUID, nil, easyjson.NewJSONObject(), false, ownerUID))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(ownerUID, podID, nil, easyjson.NewJSONObject(), false, podID))
					}
				case replicaSetKind:
					if _, ok = replicasets[ctx.Domain.CreateObjectIDWithDomain(ctx.Domain.Name(), ownerUID, false)]; ok {
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(podID, ownerUID, common.ErrorPropagateLinkTags, easyjson.NewJSONObject(), false, ownerUID))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(ownerUID, podID, common.ErrorRelyLinkTags, easyjson.NewJSONObject(), false, podID))
					}
				case daemonSetKind:
				case nodeKind:
				}
			}

			system.MsgOnErrorReturn(ctx.Signal(sfPlugins.AutoSignalSelect, common.PropagateErrorFunctionName, podID, nil, nil))
		}
		for replicasetID, replicaSet := range replicasets {
			if replicaSet.ReqError == nil {
				ownerUID, ok := replicaSet.ReqReply.GetByPath("data.body.ownerUID").AsString()
				if ok {
					for deploymentID := range deployments {
						if ctx.Domain.CreateObjectIDWithDomain(ctx.Domain.Name(), ownerUID, false) == deploymentID {
							system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(replicasetID, deploymentID, common.ErrorPropagateLinkTags, easyjson.NewJSONObject(), false, deploymentID))
							system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(deploymentID, replicasetID, common.ErrorRelyLinkTags, easyjson.NewJSONObject(), false, replicasetID))
						}
					}
				}
			}
		}
		clusterBody := easyjson.NewJSONObject()
		clusterBody.SetByPath("resources", easyjson.NewJSON(res))
		system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(clusterID, clusterBody, false))
	}

	adapterUpdateStatus(dbc)
	system.MsgOnErrorReturn(ctx.Signal(sfPlugins.AutoSignalSelect, postProcessFnName, ctx.Self.ID, nil, nil))
}

func status(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	om := sfMediators.NewOpMediator(ctx)
	data := easyjson.NewJSONObjectWithKeyValue("status", easyjson.NewJSON("ok"))
	om.AggregateOpMsg(sfMediators.OpMsgOk(data)).Reply()
}
