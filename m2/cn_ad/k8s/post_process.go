package main

import (
	"context"
	"strings"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var recalculateShadowLinksIntervalSec = system.GetEnvMustProceed("RECALCULATE_SHADOW_INTERVAL_SEC", 20)

func postProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()
	payload := ctx.Payload
	operation, ok := payload.GetByPath("operation").AsString()
	if !ok {
		le.Errorf(logCtx, "operation not found in payload")
		return
	}

	if operation == "link_model" {
		return
	}

	id, ok := payload.GetByPath("id").AsString()
	if !ok {
		lg.Logf(lg.ErrorLevel, "cannot get id from payload")
		return
	}

	weakDomain, ok := payload.GetByPath("domain").AsString()
	if !ok {
		lg.Logf(lg.ErrorLevel, "cannot get domain from object: %s", id)
		return
	}

	objType, ok := payload.GetByPath("type").AsString()
	if !ok {
		lg.Logf(lg.WarnLevel, "cannot get domain from object: %s", id)
		return
	}

	shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), weakDomain, ctx.Domain.GetObjectIDWithoutDomain(id))

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "cannot create db client")
		return
	}

	switch operation {
	case "link_arch_block":
		service, ok := payload.GetByPath("service").AsString()
		if !ok {
			lg.Logf(lg.ErrorLevel, "cannot get service from payload")
			return
		}
		k8sObjects, err := getK8sObjects(dbc)
		if err != nil {
			lg.Logln(lg.ErrorLevel, "cannot get k8s objects")
			return
		}
		for _, k8sObject := range k8sObjects {
			if strings.Contains(k8sObject.ImageName, service) {
				dbc.CMDB.ShadowObjectCanBeRecevier = true
				system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(shadowID, easyjson.NewJSONObject(), false, objType))
				dbc.CMDB.ShadowObjectCanBeRecevier = false
				if createShadowLink(dbc, ctx, common.ErrorPropagateLinkTags, k8sObject.ID, id, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, weakDomain) {
					le.Infof(logCtx, "Linked k8s object (type: %s) to shadow Arch Block %s", k8sObject.ObjType, service)
				}
			}
		}
	case "link_vm":
		uuid, ok := payload.GetByPath("uuid").AsString()
		if !ok {
			lg.Logf(lg.ErrorLevel, "cannot get uuid from payload %v", payload)
			return
		}
		k8sNodes, err := getNodes(dbc)
		if err != nil {
			lg.Logln(lg.ErrorLevel, "cannot get k8s nodes")
			return
		}
		for _, k8sNode := range k8sNodes {
			if uuid == k8sNode.SystemUID {
				dbc.CMDB.ShadowObjectCanBeRecevier = true
				system.MsgOnErrorReturn(dbc.CMDB.ObjectUpdate(shadowID, easyjson.NewJSONObject(), false, objType))
				dbc.CMDB.ShadowObjectCanBeRecevier = false
				if createShadowLink(dbc, ctx, common.ErrorPropagateLinkTags, k8sNode.ID, id, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, weakDomain) {
					le.Infof(logCtx, "Linked Node %s to shadow VM %s", k8sNode.ID, uuid)
				}
			}
		}
	default:
		//le.Infof(logCtx, "operation '%s' is not supported", operation)
	}
}

func shadowLinksKeeper(ctx context.Context, dbc db.DBSyncClient, runtime *statefun.Runtime) {
	ticker := time.NewTicker(time.Duration(recalculateShadowLinksIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var uidsForLink []easyjson.JSON
			nodes, err := getNodes(dbc)
			if err == nil {
				for _, node := range nodes {
					notifierPayload := easyjson.NewJSONObject()
					notifierPayload.SetByPath("domain", easyjson.NewJSON(runtime.Domain.Name()))
					notifierPayload.SetByPath("UID", easyjson.NewJSON(runtime.Domain.GetObjectIDWithoutDomain(node.ID)))
					notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_NODE))
					notifierPayload.SetByPath("operation", easyjson.NewJSON("add"))
					notifierPayload.SetByPath("systemUID", easyjson.NewJSON(node.SystemUID))
					uidsForLink = append(uidsForLink, notifierPayload)
				}
			}
			k8sObjects, err := getK8sObjects(dbc)
			if err == nil {
				for _, k8sObject := range k8sObjects {
					notifierPayload := easyjson.NewJSONObject()
					notifierPayload.SetByPath("domain", easyjson.NewJSON(runtime.Domain.Name()))
					notifierPayload.SetByPath("UID", easyjson.NewJSON(runtime.Domain.GetObjectIDWithoutDomain(k8sObject.ID)))
					notifierPayload.SetByPath("operation", easyjson.NewJSON("add"))
					notifierPayload.SetByPath("containersImage", easyjson.NewJSON(k8sObject.ImageName))
					switch k8sObject.ObjType {
					case types.TYPE_FOLIAGE_POD:
						notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_POD))
					case types.TYPE_FOLIAGE_DEPLOYMENT:
						notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_DEPLOYMENT))
					}
					uidsForLink = append(uidsForLink, notifierPayload)
				}
			}
			common.NotifyAdapters(runtime, dbc, uidsForLink...)
		}
	}
}

func createShadowLink(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, tags []string, fromId, toId, toType, targetDomain string) bool {
	shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), targetDomain, ctx.Domain.GetObjectIDWithoutDomain(toId))

	dbc.CMDB.ShadowObjectCanBeRecevier = true
	system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, toType))
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

	nodeIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_NODE, ".*[l:type('__object')]")
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

				imageName := objData.GetByPath("body.containersImage").AsStringDefault("")
				if imageName != "" {
					objects = append(objects, K8sObject{
						ID:        id,
						ObjType:   _type,
						ImageName: imageName,
					})
				}
			}
		}
	}

	getAll(types.TYPE_FOLIAGE_POD)
	getAll(types.TYPE_FOLIAGE_DEPLOYMENT)

	return objects, nil
}

func normalizeImageName(fullImage string) string {
	lastSlash := strings.LastIndex(fullImage, "/")
	if lastSlash != -1 {
		fullImage = fullImage[lastSlash+1:]
	}

	colonIdx := strings.Index(fullImage, ":")
	if colonIdx != -1 {
		fullImage = fullImage[:colonIdx]
	}

	return strings.ToLower(fullImage)
}
