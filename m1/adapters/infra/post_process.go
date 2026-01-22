package main

import (
	"context"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

func infraPostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()
	payload := ctx.Payload

	operation, ok := payload.GetByPath("operation").AsString()
	if !ok {
		le.Errorf(logCtx, "operation not found in payload")
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

		if objType == types.TYPE_FOLIAGE_NODE {
			handleNodeSignal(dbc, ctx, payload)
		}

	case "delete":
		objType, ok := payload.GetByPath("type").AsString()
		if !ok {
			return
		}

		if objType == types.TYPE_FOLIAGE_NODE {
			uid, ok := payload.GetByPath("UID").AsString()
			if !ok {
				return
			}
			// Delete shadow object
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), "m2", uid)
			_ = dbc.CMDB.ObjectDelete(shadowID)
			le.Infof(logCtx, "Deleted shadow Node %s", shadowID)
		}

	default:
		//le.Debugf(logCtx, "operation '%s' is not supported", operation)
	}
}

func handleNodeSignal(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, payload *easyjson.JSON) {
	le := lg.GetLogger()
	logCtx := context.Background()

	nodeUID, ok := payload.GetByPath("UID").AsString()
	if !ok {
		le.Errorf(logCtx, "cannot get UID from payload")
		return
	}

	systemUID, ok := payload.GetByPath("systemUID").AsString()
	if !ok {
		le.Errorf(logCtx, "cannot get systemUID from payload")
		return
	}
	systemUID = strings.ToLower(systemUID)

	// Find VMs with matching product_uuid
	vms, err := getVirtualMachines(dbc)
	if err != nil {
		le.Errorf(logCtx, "cannot get virtual machines: %v", err)
		return
	}

	for _, vm := range vms {
		if vm.ProductUUID == systemUID {
			// Create shadow Node object
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), "m2", nodeUID)

			dbc.CMDB.ShadowObjectCanBeRecevier = true
			system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, types.TYPE_FOLIAGE_NODE))
			dbc.CMDB.ShadowObjectCanBeRecevier = false

			// Link VM → shadow(Node)
			err := dbc.CMDB.ObjectsLinkUpdate(vm.ID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID)
			if err == nil {
				le.Infof(logCtx, "Linked VM %s to shadow Node %s (systemUID: %s)", vm.ID, shadowID, systemUID)
			}
		}
	}
}

type VirtualMachine struct {
	ID          string
	ProductUUID string
}

func getVirtualMachines(dbc db.DBSyncClient) ([]VirtualMachine, error) {
	var vms []VirtualMachine

	vmIDs, err := dbc.Query.JPGQLCtraQuery(types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, ".*[l:type('__object')]")
	if err != nil {
		return nil, err
	}

	for _, vmID := range vmIDs {
		objData, err := dbc.CMDB.ObjectRead(vmID)
		if err != nil {
			continue
		}

		productUUID := objData.GetByPath("body.sources.configuration.uuid").AsStringDefault("")

		vms = append(vms, VirtualMachine{
			ID:          vmID,
			ProductUUID: strings.ToLower(productUUID),
		})
	}

	return vms, nil
}
