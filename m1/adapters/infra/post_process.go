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

	m2Nodes, err := getM2Nodes(ctx, dbc)
	if err != nil {
		le.Errorf(logCtx, "infraPostProcess: cannot get m2 nodes %v", err)
		return
	}

	for _, node := range m2Nodes {
		if vmID, ok := virtualMachines[node.SystemUID]; ok {
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), common.ModelM2, ctx.Domain.GetObjectIDWithoutDomain(node.ID))
			dbc.CMDB.ShadowObjectCanBeRecevier = true
			err = dbc.CMDB.ObjectUpdate(shadowID, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_NODE)
			dbc.CMDB.ShadowObjectCanBeRecevier = false
			if err != nil {
				le.Errorf(logCtx, "archModelPostProcess: cannot create shadow object %s: %v", shadowID, err)
				return
			}
			// Link virtualMachine → shadow(Node)
			system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(vmID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID))
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

func getM2Nodes(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient) ([]K8sNode, error) {
	var nodes []K8sNode

	ids, err := dbc.Query.JPGQLCtraQuery(ctx.Domain.CreateObjectIDWithDomain(common.ModelM2, types.TYPE_FOLIAGE_NODE, true), common.AllObjectsQuery)
	if err == nil {
		for _, id := range ids {
			objData, err := dbc.CMDB.ObjectRead(id)
			if err != nil {
				continue
			}
			systemUUID, ok := objData.GetByPath("body.systemUID").AsString()
			if ok {
				nodes = append(nodes, K8sNode{
					ID:        id,
					SystemUID: systemUUID,
				})
			}
		}
	}

	return nodes, nil
}
