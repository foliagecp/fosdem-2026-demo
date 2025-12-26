package main

import (
	"context"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfMediators "github.com/foliagecp/sdk/statefun/mediator"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

func buildLinks(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	funcCtx := ctx.GetFunctionContext()
	rebuildVersion := funcCtx.GetByPath("rebuildVersion").AsNumericDefault(0)
	lg.GetLogger().Infof(context.TODO(), "function 'buildLinks' has started, current version: %v", rebuildVersion)
	rebuildVersion++
	funcCtx.SetByPath("rebuildVersion", easyjson.NewJSON(rebuildVersion))
	ctx.SetFunctionContext(funcCtx)

	const foliageReadFunc = "functions.cmdb.api.object.read"
	le := lg.GetLogger()
	logCtx := context.Background()
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		le.Errorf(logCtx, "buildLinks: cannot create db client: %v", err)
		return
	}
	clusters, err := dbc.Query.JPGQLCtraQuery(m2.CLUSTER_TYPE, ".*[l:type('__object')]")
	if err != nil {
		le.Errorf(logCtx, "buildLinks: cannot query clusters: %v", err)
		return
	}
	for _, clusterID := range clusters {
		nodes, err := ctx.ObjectRequest(
			sfPlugins.AutoRequestSelect,
			sfPlugins.NewLinkQuery(m2.NODE_TYPE),
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
			sfPlugins.NewLinkQuery(m2.POD_TYPE),
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
			sfPlugins.NewLinkQuery(m2.DEPLOYMENT_TYPE),
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
			sfPlugins.NewLinkQuery(m2.REPLICATION_SET_TYPE),
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
				nodeName, ok := pod.ReqReply.GetByPath("data.body.nodeName").AsString()
				if ok {
					nodeID, ok := nodesNameMap[nodeName]
					if ok {
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(podID, nodeID, nil, easyjson.NewJSONObject(), false, m2.NODE_TYPE))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(nodeID, podID, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
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
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(podID, ownerUID, nil, easyjson.NewJSONObject(), false, m2.DEPLOYMENT_TYPE))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(ownerUID, podID, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
					}
				case replicaSetKind:
					if _, ok = replicasets[ctx.Domain.CreateObjectIDWithDomain(ctx.Domain.Name(), ownerUID, false)]; ok {
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(podID, ownerUID, nil, easyjson.NewJSONObject(), false, m2.REPLICATION_SET_TYPE))
						system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(ownerUID, podID, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
					}
				case daemonSetKind:
				case nodeKind:
				}
			}
		}
	}
}

func kickAdapters(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	for _, dm := range ctx.Domain.GetWeakClusterDomains() {
		if dm != ctx.Domain.Name() {
			system.MsgOnErrorReturn(
				ctx.Signal(
					sfPlugins.AutoSignalSelect,
					"notify_adapters", //TODO universal func for every domain?
					ctx.Self.ID,
					ctx.Payload,
					nil,
				),
			)
		}
	}
}

func status(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	om := sfMediators.NewOpMediator(ctx)
	data := easyjson.NewJSONObjectWithKeyValue("status", easyjson.NewJSON("ok"))
	om.AggregateOpMsg(sfMediators.OpMsgOk(data)).Reply()
}
