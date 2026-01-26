package main

import (
	"context"
	"strings"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m3/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var recalculateShadowLinksIntervalSec = system.GetEnvMustProceed("RECALCULATE_SHADOW_INTERVAL_SEC", 20)

func archModelPostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()
	payload := ctx.Payload

	operation, ok := payload.GetByPath("operation").AsString()
	if !ok {
		le.Errorf(logCtx, "archModelPostProcess: operation not found in payload")
		return
	}

	if operation == "link_model" {
		return
	}

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "cannot create db client")
		return
	}

	switch operation {
	case "add":
		objType, ok := payload.GetByPath("type").AsString()
		if !ok {
			le.Errorf(logCtx, "type not found in payload")
			return
		}

		if objType == types.TYPE_FOLIAGE_POD || objType == types.TYPE_FOLIAGE_DEPLOYMENT {
			handleK8sObjectSignal(dbc, ctx, payload, objType)
		}

	case "delete":
		objType, ok := payload.GetByPath("type").AsString()
		if !ok {
			return
		}

		if objType == types.TYPE_FOLIAGE_POD || objType == types.TYPE_FOLIAGE_DEPLOYMENT {
			uid, ok := payload.GetByPath("UID").AsString()
			if !ok {
				return
			}
			// Delete shadow object
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), "m2", uid)
			_ = dbc.CMDB.ObjectDelete(shadowID)
			le.Infof(logCtx, "Deleted shadow %s %s", objType, shadowID)
		}

	default:
		//le.Debugf(logCtx, "operation '%s' is not supported", operation)
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
			blocks, err := getArchBlocks(dbc)
			if err == nil {
				for _, block := range blocks {
					notifierPayload := easyjson.NewJSONObject()
					notifierPayload.SetByPath("domain", easyjson.NewJSON(runtime.Domain.Name()))
					notifierPayload.SetByPath("id", easyjson.NewJSON(runtime.Domain.GetObjectIDWithoutDomain(block.ID)))
					notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
					notifierPayload.SetByPath("operation", easyjson.NewJSON("link_arch_block"))
					notifierPayload.SetByPath("service", easyjson.NewJSON(block.ServiceName))
					uidsForLink = append(uidsForLink, notifierPayload)
				}
			}
			common.NotifyAdapters(runtime, dbc, uidsForLink...)
		}
	}
}

func handleK8sObjectSignal(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, payload *easyjson.JSON, objType string) {
	le := lg.GetLogger()
	logCtx := context.Background()

	uid, ok := payload.GetByPath("UID").AsString()
	if !ok {
		le.Debugf(logCtx, "handleK8sObjectSignal: cannot get UID from payload")
		return
	}

	containersImage, ok := payload.GetByPath("containersImage").AsString()
	if !ok {
		le.Debugf(logCtx, "handleK8sObjectSignal: cannot get containersImage from payload, skipping")
		return
	}

	domain, ok := payload.GetByPath("domain").AsString()
	if !ok {
		le.Debugf(logCtx, "handleK8sObjectSignal: cannot get domain from payload")
		return
	}

	// Normalize image name for matching
	imageName := normalizeImageName(containersImage)
	if imageName == "" {
		return
	}

	// Find ArchBlocks with matching service name
	archBlocks, err := getArchBlocks(dbc)
	if err != nil {
		le.Errorf(logCtx, "handleK8sObjectSignal: cannot get arch blocks: %v", err)
		return
	}

	for _, block := range archBlocks {
		if strings.Contains(imageName, block.ServiceName) {
			// Create shadow K8s object
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), domain, ctx.Domain.GetObjectIDWithoutDomain(uid))

			dbc.CMDB.ShadowObjectCanBeRecevier = true
			system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, objType))
			dbc.CMDB.ShadowObjectCanBeRecevier = false

			// Link ArchBlock → shadow(Pod/Deployment)
			system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(block.ID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID))
		}
	}
}

type ArchBlock struct {
	ID          string
	ServiceName string
}

func getArchBlocks(dbc db.DBSyncClient) ([]ArchBlock, error) {
	var blocks []ArchBlock

	blockIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, ".*[l:type('__object')]")
	if err != nil {
		return nil, err
	}

	for _, blockID := range blockIDs {
		objData, err := dbc.CMDB.ObjectRead(blockID)
		if err != nil {
			continue
		}

		serviceName, ok := objData.GetByPath("body.service").AsString()
		if !ok {
			lg.Logf(lg.ErrorLevel, "cant get field service from arch block: %s", blockID)
			continue
		}

		blocks = append(blocks, ArchBlock{
			ID:          blockID,
			ServiceName: strings.ToLower(serviceName),
		})
	}

	return blocks, nil
}

func normalizeImageName(fullImage string) string {
	// Extract image name from full path like gcr.io/project/frontend:v1.2
	lastSlash := strings.LastIndex(fullImage, "/")
	if lastSlash != -1 {
		fullImage = fullImage[lastSlash+1:]
	}

	// Remove tag
	colonIdx := strings.Index(fullImage, ":")
	if colonIdx != -1 {
		fullImage = fullImage[:colonIdx]
	}

	return strings.ToLower(fullImage)
}
