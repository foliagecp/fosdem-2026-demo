package main

import (
	"context"
	"strings"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	"github.com/foliagecp/sdk/statefun/system"
)

var linksToM3UpdateIntervalSec = system.GetEnvMustProceed("LINKS_TO_M3_UPDATE_INTERVAL_SEC", 2)

func (w *Watcher) linkToArchitectureBlocks(ctx context.Context, runtime *statefun.Runtime) {
	time.Sleep(15 * time.Second)

	interval := time.Duration(linksToM3UpdateIntervalSec) * time.Second
	if interval <= 0 {
		interval = 20 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.performArchitectureLinking(runtime)
		}
	}
}

func (w *Watcher) performArchitectureLinking(runtime *statefun.Runtime) {
	le := lg.GetLogger()
	dbc, err := db.NewDBSyncClientFromRequestFunction(runtime.Request)
	if err != nil {
		le.Errorf(context.Background(), "cannot create db client for architecture linking: %v", err)
		return
	}

	le.Infof(context.Background(), "Starting architecture linking scan...")

	archBlocks, err := findArchitectureBlocks(dbc, runtime)
	if err != nil {
		le.Warnf(context.Background(), "failed to find architecture blocks: %v", err)
		return
	}

	le.Infof(context.Background(), "Found %d architecture blocks", len(archBlocks))

	k8sObjects, err := w.getK8sObjects(dbc)
	if err != nil {
		le.Warnf(context.Background(), "failed to get k8s objects: %v", err)
		return
	}

	le.Infof(context.Background(), "Found %d k8s objects (pods+deployments)", len(k8sObjects))

	linksCreated := 0
	for _, k8sObj := range k8sObjects {
		for _, archBlock := range archBlocks {
			if shouldLink(k8sObj, archBlock) {
				if createShadowLink(dbc, runtime, k8sObj, archBlock) {
					linksCreated++
					le.Infof(context.Background(), "Linked %s (%s) to %s (image match: %s)",
						k8sObj.ID, k8sObj.ObjType, archBlock.Name, k8sObj.ImageName)
				}
			}
		}
	}

	le.Infof(context.Background(), "Architecture linking scan complete. Created %d new links", linksCreated)
}

type K8sObject struct {
	ID        string
	ObjType   string
	ImageName string
}

type ArchBlock struct {
	ID          string
	Name        string
	ServiceName string
}

func findArchitectureBlocks(dbc db.DBSyncClient, runtime *statefun.Runtime) ([]ArchBlock, error) {
	var blocks []ArchBlock

	archBlockTypeID := runtime.Domain.CreateObjectIDWithDomain("m3", types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, true)

	blockIDs, err := dbc.Query.JPGQLCtraQuery(archBlockTypeID, ".*[l:type('__object')]")
	if err != nil {
		return nil, err
	}

	for _, blockID := range blockIDs {
		objData, err := dbc.CMDB.ObjectRead(blockID)
		if err != nil {
			continue
		}

		name, _ := objData.GetByPath("body.name").AsString()
		serviceName, _ := objData.GetByPath("body.service").AsString()

		blocks = append(blocks, ArchBlock{
			ID:          blockID,
			Name:        name,
			ServiceName: serviceName,
		})
	}

	return blocks, nil
}

func (w *Watcher) getK8sObjects(dbc db.DBSyncClient) ([]K8sObject, error) {
	const allObjectsQuery = ".*[l:type('__object')]"
	var objects []K8sObject

	podIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_POD, allObjectsQuery)
	if err == nil {
		for _, podID := range podIDs {
			objData, err := dbc.CMDB.ObjectRead(podID)
			if err != nil {
				continue
			}

			imageName := objData.GetByPath("body.containersImage").AsStringDefault("")
			if imageName != "" {
				objects = append(objects, K8sObject{
					ID:        podID,
					ObjType:   types.TYPE_FOLIAGE_POD,
					ImageName: imageName,
				})
			}
		}
	}

	deplIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_DEPLOYMENT, allObjectsQuery)
	if err == nil {
		for _, deplID := range deplIDs {
			objData, err := dbc.CMDB.ObjectRead(deplID)
			if err != nil {
				continue
			}

			imageName := objData.GetByPath("body.containersImage").AsStringDefault("")
			if imageName != "" {
				objects = append(objects, K8sObject{
					ID:        deplID,
					ObjType:   types.TYPE_FOLIAGE_DEPLOYMENT,
					ImageName: imageName,
				})
			}
		}
	}

	return objects, nil
}

func shouldLink(k8sObj K8sObject, archBlock ArchBlock) bool {
	imageName := normalizeImageName(k8sObj.ImageName)

	if archBlock.ServiceName != "" && strings.Contains(imageName, archBlock.ServiceName) {
		return true
	}

	return false
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

func createShadowLink(dbc db.DBSyncClient, runtime *statefun.Runtime, k8sObj K8sObject, archBlock ArchBlock) bool {
	shadowID := runtime.Domain.CreateCustomShadowId(runtime.Domain.HubDomainName(), "m3", runtime.Domain.GetObjectIDWithoutDomain(archBlock.ID))

	dbc.CMDB.ShadowObjectCanBeRecevier = true
	system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
	dbc.CMDB.ShadowObjectCanBeRecevier = false

	err := dbc.CMDB.ObjectsLinkUpdate(k8sObj.ID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID)

	return err == nil
}
