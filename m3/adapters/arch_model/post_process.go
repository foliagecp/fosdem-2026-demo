package main

import (
	"context"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m3/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

func archModelPostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		le.Errorf(logCtx, "archModelPostProcess: db.NewDBSyncClientFromRequestFunction error: %v", err)
		return
	}

	archBlocks, err := getArchBlocks(dbc)
	if err != nil {
		le.Errorf(logCtx, "archModelPostProcess: getArchBlocks error: %v", err)
		return
	}
	k8sObjectsFromM2, err := getM2Objects(ctx, dbc)
	if err != nil {
		le.Errorf(logCtx, "archModelPostProcess: getM2Objects error: %v", err)
		return
	}

	k8sObjectsShadow, err := getM2ShadowObjects(ctx, dbc)
	if err != nil {
		le.Errorf(logCtx, "archModelPostProcess: getM2Objects error: %v", err)
		return
	}

	res := reconcile(k8sObjectsFromM2, k8sObjectsShadow)

	for _, id := range res.ToDelete {
		dbc.CMDB.ShadowObjectCanBeRecevier = true
		system.MsgOnErrorReturn(dbc.CMDB.ObjectDelete(ctx.Domain.CreateCustomShadowId(ctx.Domain.Name(), common.ModelM2, ctx.Domain.GetObjectIDWithoutDomain(id))))
		dbc.CMDB.ShadowObjectCanBeRecevier = false
	}

	for _, k8sObject := range res.ToUpsert {
		var archBlockID string
		var ok bool
		switch k8sObject.ObjType {
		case types.TYPE_FOLIAGE_POD:
			if archBlockID, ok = archBlocks[k8sObject.LabelsApp]; !ok {
				le.Warnf(logCtx, "archModelPostProcess: k8sObject.LabelsApp %s not found in archBlocks", k8sObject.LabelsApp)
				continue
			}
		case types.TYPE_FOLIAGE_DEPLOYMENT:
			if archBlockID, ok = archBlocks[k8sObject.Name]; !ok {
				le.Warnf(logCtx, "archModelPostProcess: k8sObject.Name %s not found in archBlocks", k8sObject.Name)
				continue
			}
		}

		shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), common.ModelM2, ctx.Domain.GetObjectIDWithoutDomain(k8sObject.ID))

		dbc.CMDB.ShadowObjectCanBeRecevier = true
		err = dbc.CMDB.ObjectUpdate(shadowID, easyjson.NewJSONObject(), false, ctx.Domain.CreateObjectIDWithDomain(ctx.Domain.Name(), k8sObject.ObjType, true))
		dbc.CMDB.ShadowObjectCanBeRecevier = false
		if err != nil {
			le.Errorf(logCtx, "archModelPostProcess: cannot create shadow object %s: %v", shadowID, err)
			return
		}
		// Link archBlock → shadow(Pod/Deployment)
		system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(archBlockID, shadowID, common.ErrorPropagateLinkTags, easyjson.NewJSONObject(), false, shadowID))
	}
}

func getArchBlocks(dbc db.DBSyncClient) (map[string]string, error) {
	le := lg.GetLogger()
	logCtx := context.Background()

	blockIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, common.AllObjectsQuery)
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

type K8sObject struct {
	ID        string
	ObjType   string
	ImageName string
	Name      string
	LabelsApp string
}

func getM2Objects(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient) ([]K8sObject, error) {
	var objects []K8sObject

	getAll := func(_type string) {
		ids, err := dbc.Query.JPGQLCtraQuery(ctx.Domain.CreateObjectIDWithDomain(common.ModelM2, _type, true), common.AllObjectsQuery)
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

func getM2ShadowObjects(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient) ([]string, error) {
	var objects []string

	getAll := func(_type string) {
		ids, err := dbc.Query.JPGQLCtraQuery(_type, common.AllObjectsQuery)
		if err == nil {
			//
			for _, id := range ids {
				// m3/m2#id -> m2/id
				// m2/id
				clearID := ctx.Domain.GetObjectIDByShadowObjectID(id)
				if err == nil {
					objects = append(objects, clearID)
				}
			}
		}
	}

	getAll(types.TYPE_FOLIAGE_POD)
	getAll(types.TYPE_FOLIAGE_DEPLOYMENT)

	return objects, nil
}

type ReconcileResult struct {
	ToUpsert []K8sObject
	ToDelete []string
}

func reconcile(m2 []K8sObject, m3 []string) ReconcileResult {
	m2Set := make(map[string]struct{}, len(m2))
	for _, o := range m2 {
		m2Set[o.ID] = struct{}{}
	}

	var res ReconcileResult

	res.ToUpsert = append(res.ToUpsert, m2...)

	for _, id := range m3 {
		if _, ok := m2Set[id]; !ok {
			res.ToDelete = append(res.ToDelete, id)
		}
	}

	return res
}
