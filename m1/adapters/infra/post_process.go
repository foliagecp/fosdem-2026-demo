package main

import (
	"context"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

func infraPostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		le.Errorf(logCtx, "infraPostProcess: cannot create db client %v", err)
		return
	}

	virtualMachines, err := getVirtualMachines(dbc)
	if err != nil {
		le.Errorf(logCtx, "infraPostProcess: cannot get virtual machines %v", err)
		return
	}

	m2Nodes, err := getNodesByDomain(ctx, dbc, common.ModelM2)
	if err != nil {
		le.Errorf(logCtx, "infraPostProcess: cannot get m2 nodes %v", err)
		return
	}

	m1ShadowNodes, err := getNodesByDomain(ctx, dbc, ctx.Domain.Name())
	if err != nil {
		le.Errorf(logCtx, "infraPostProcess: cannot get m1 nodes %v", err)
		return
	}

	res := reconcileShadowNodes(m2Nodes, m1ShadowNodes)

	for _, node := range res.ToDelete {
		dbc.CMDB.ShadowObjectCanBeRecevier = true
		system.MsgOnErrorReturn(dbc.CMDB.ObjectDelete(ctx.Domain.CreateCustomShadowId(ctx.Domain.Name(), common.ModelM2, node.ID)))
		dbc.CMDB.ShadowObjectCanBeRecevier = false
	}

	for _, node := range res.ToUpsert {
		if vmID, ok := virtualMachines[node.SystemUID]; ok {
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), common.ModelM2, node.ID)
			dbc.CMDB.ShadowObjectCanBeRecevier = true
			err = dbc.CMDB.ObjectUpdate(shadowID, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_NODE)
			dbc.CMDB.ShadowObjectCanBeRecevier = false
			if err != nil {
				le.Errorf(logCtx, "infraPostProcess: cannot create shadow object %s: %v", shadowID, err)
				return
			}
			// Link virtualMachine → shadow(Node)
			dbc.CMDB.ShadowObjectCanBeRecevier = true
			system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(vmID, shadowID, common.ErrorPropagateLinkTags, easyjson.NewJSONObject(), false, shadowID))
			dbc.CMDB.ShadowObjectCanBeRecevier = false
		}
	}
}

func getVirtualMachines(dbc db.DBSyncClient) (map[string]string, error) {
	vmIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, common.AllObjectsQuery)
	if err != nil {
		return nil, err
	}

	vms := make(map[string]string, len(vmIDs))

	for _, vmID := range vmIDs {
		objData, err := dbc.CMDB.ObjectRead(vmID)
		if err != nil {
			lg.Logf(lg.ErrorLevel, "getVirtualMachines: cannot read object %s: %v", vmID, err)
			continue
		}

		productUUID, ok := objData.GetByPath("body.uuid").AsString()
		if ok {
			vms[productUUID] = vmID
		}
	}
	return vms, nil
}

type K8sNode struct {
	ID        string
	SystemUID string
}

func getNodesByDomain(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, dm string) ([]K8sNode, error) {
	var nodes []K8sNode

	ids, err := dbc.Query.JPGQLCtraQuery(ctx.Domain.CreateObjectIDWithDomain(dm, types.TYPE_FOLIAGE_NODE, true), common.AllObjectsQuery)
	if err == nil {
		for _, id := range ids {

			var clearedID string
			var objData easyjson.JSON
			var err error
			if dm == ctx.Domain.Name() {
				clearedID = ctx.Domain.GetObjectIDWithoutDomain(id)
			} else {
				objData, err = dbc.CMDB.ObjectRead(id)
				if err != nil {
					continue
				}
				_, clearedID, _ = ctx.Domain.GetShadowObjectDomainAndID(id)
			}
			nodes = append(nodes, K8sNode{
				ID:        clearedID,
				SystemUID: objData.GetByPath("body.systemUID").AsStringDefault(""),
			})
		}
	}

	return nodes, nil
}

type ReconcileResult struct {
	ToUpsert []K8sNode
	ToDelete []K8sNode
}

func reconcileShadowNodes(m2, m1 []K8sNode) ReconcileResult {
	m2Set := make(map[string]struct{}, len(m2))
	for _, n := range m2 {
		m2Set[n.ID] = struct{}{}
	}

	var res ReconcileResult

	res.ToUpsert = append(res.ToUpsert, m2...)

	for _, n := range m1 {
		if _, ok := m2Set[n.ID]; !ok {
			res.ToDelete = append(res.ToDelete, n)
		}
	}

	return res
}
