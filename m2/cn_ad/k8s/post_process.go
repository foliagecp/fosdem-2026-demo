package main

import (
	"context"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

func k8sInfrastructurePostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		le.Errorf(logCtx, "k8sInfrastructurePostProcess: cannot create db client %v", err)
		return
	}

	virtualMachines, err := getVirtualMachinesFromM1(ctx, dbc)
	if err == nil {
		nodes, err := getNodes(dbc)
		if err == nil {
			for _, node := range nodes {
				if vmID, ok := virtualMachines[node.SystemUID]; ok {
					createShadowLink(dbc, ctx, common.ErrorPropagateLinkTags, node.ID, vmID, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, common.ModelM1)
				}
			}
		} else {
			le.Errorf(logCtx, "k8sInfrastructurePostProcess: cannot get nodes %v", err)
		}
	} else {
		le.Errorf(logCtx, "k8sInfrastructurePostProcess: cannot get virtual machines %v", err)
	}

	archBlocks, err := getArchBlocksFromM3(ctx, dbc)
	if err == nil {
		k8sObjects, err := getK8sObjects(dbc)
		if err == nil {
			for _, k8sObject := range k8sObjects {
				var archBlockID string
				var ok bool
				switch k8sObject.ObjType {
				case types.TYPE_FOLIAGE_POD:
					if archBlockID, ok = archBlocks[k8sObject.LabelsApp]; !ok {
						le.Warnf(logCtx, "k8sInfrastructurePostProcess: cannot find arch block for pod %v", k8sObject.LabelsApp)
					}
				case types.TYPE_FOLIAGE_DEPLOYMENT:
					if archBlockID, ok = archBlocks[k8sObject.Name]; !ok {
						le.Warnf(logCtx, "k8sInfrastructurePostProcess: cannot find arch block for deployment %v", k8sObject.Name)
					}
				}
				// find by container image name
				if !ok {
					for serviceName := range archBlocks {
						if strings.Contains(k8sObject.ImageName, serviceName) {
							archBlockID = archBlocks[serviceName]
							break
						}
					}
				}
				if archBlockID != "" {
					createShadowLink(dbc, ctx, common.ErrorPropagateLinkTags, k8sObject.ID, archBlockID, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, common.ModelM3)
				}
			}
		}
	}

	common.PostProcessNotifier(dbc, ctx)
}

func createShadowLink(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, tags []string, fromId, toId, toType, targetDomain string) bool {
	shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), targetDomain, ctx.Domain.GetObjectIDWithoutDomain(toId))

	dbc.CMDB.ShadowObjectCanBeRecevier = true
	system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(shadowID, easyjson.NewJSONObject(), false, toType))
	dbc.CMDB.ShadowObjectCanBeRecevier = false

	err := dbc.CMDB.ObjectsLinkUpdate(fromId, shadowID, tags, easyjson.NewJSONObject(), false, shadowID)

	return err == nil
}

type Node struct {
	ID        string
	SystemUID string
}

func getNodes(dbc db.DBSyncClient) ([]Node, error) {
	var nodes []Node

	nodeIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_NODE, common.AllObjectsQuery)
	if err != nil {
		return nil, err
	}

	for _, nodeID := range nodeIDs {
		objData, err := dbc.CMDB.ObjectRead(nodeID)
		if err != nil {
			continue
		}

		systemUID := objData.GetByPath("body.systemUID").AsStringDefault("")

		nodes = append(nodes, Node{
			ID:        nodeID,
			SystemUID: strings.ToLower(systemUID),
		})
	}

	return nodes, nil
}

type K8sObject struct {
	ID        string
	ObjType   string
	ImageName string
	Name      string
	LabelsApp string
}

func getK8sObjects(dbc db.DBSyncClient) ([]K8sObject, error) {
	const allObjectsQuery = ".*[l:type('__object')]"
	var objects []K8sObject

	getAll := func(_type string) {
		ids, err := dbc.Query.JPGQLCtraQuery(_type, allObjectsQuery)
		if err == nil {
			for _, id := range ids {
				objData, err := dbc.CMDB.ObjectRead(id)
				if err != nil {
					continue
				}
				objects = append(objects, K8sObject{
					ID:        id,
					ObjType:   _type,
					ImageName: objData.GetByPath("body.containersImage").AsStringDefault(""),
					Name:      objData.GetByPath("body.name").AsStringDefault(""),
					LabelsApp: objData.GetByPath("body.labelsApp").AsStringDefault(""),
				})
			}
		}
	}

	getAll(types.TYPE_FOLIAGE_POD)
	getAll(types.TYPE_FOLIAGE_DEPLOYMENT)

	return objects, nil
}

func getVirtualMachinesFromM1(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient) (map[string]string, error) {
	le := lg.GetLogger()
	logCtx := context.Background()

	vmIDs, err := dbc.Query.JPGQLCtraQuery(
		ctx.Domain.CreateObjectIDWithDomain(common.ModelM1, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, true), common.AllObjectsQuery)
	if err != nil {
		return nil, err
	}

	vms := make(map[string]string, len(vmIDs))

	for _, vmID := range vmIDs {
		objData, err := dbc.CMDB.ObjectRead(vmID)
		if err != nil {
			le.Errorf(logCtx, "getVirtualMachines: cannot read object %s: %v", vmID, err)
			continue
		}

		productUUID, ok := objData.GetByPath("body.uuid").AsString()
		if ok {
			vms[productUUID] = vmID
		}
	}
	return vms, nil
}

func getArchBlocksFromM3(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient) (map[string]string, error) {
	le := lg.GetLogger()
	logCtx := context.Background()

	blockIDs, err := dbc.Query.JPGQLCtraQuery(
		ctx.Domain.CreateObjectIDWithDomain(common.ModelM3, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, true), common.AllObjectsQuery)
	if err != nil {
		le.Errorf(logCtx, "getArchBlocks: cannot get arch blocks: %v", err)
		return nil, err
	}

	blocks := make(map[string]string, len(blockIDs))

	for _, blockID := range blockIDs {
		objData, err := dbc.CMDB.ObjectRead(blockID)
		if err != nil {
			le.Errorf(logCtx, "getArchBlocks: cannot read arch block %s: %v", blockID, err)
			continue
		}

		serviceName, ok := objData.GetByPath("body.service").AsString()
		if !ok {
			le.Warnf(logCtx, "getArchBlocks: cannot get arch block %s: service name not found", blockID)
			continue
		}

		blocks[strings.ToLower(serviceName)] = blockID
	}

	return blocks, nil
}
