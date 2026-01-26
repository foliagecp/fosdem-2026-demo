package main

import (
	"context"
	"strings"
	"time"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var recalculateShadowLinksIntervalSec = system.GetEnvMustProceed("RECALCULATE_SHADOW_INTERVAL_SEC", 20)

func infraPostProcess(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	le := lg.GetLogger()
	logCtx := context.Background()
	payload := ctx.Payload

	operation, ok := payload.GetByPath("operation").AsString()
	if !ok {
		le.Errorf(logCtx, "infraPostProcess: operation not found in payload")
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

func shadowLinksKeeper(ctx context.Context, dbc db.DBSyncClient, runtime *statefun.Runtime) {
	ticker := time.NewTicker(time.Duration(recalculateShadowLinksIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			vms, err := getVirtualMachines(dbc)
			if err != nil {
				lg.Logln(lg.ErrorLevel, "shadowLinksKeeper: cannot get virtual machines")
				continue
			}
			var vmsForLink []easyjson.JSON
			for _, vm := range vms {
				notifierPayload := easyjson.NewJSONObject()
				notifierPayload.SetByPath("domain", easyjson.NewJSON(runtime.Domain.Name()))
				notifierPayload.SetByPath("id", easyjson.NewJSON(vm.ID))
				notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE))
				notifierPayload.SetByPath("operation", easyjson.NewJSON("link_vm"))
				notifierPayload.SetByPath("uuid", easyjson.NewJSON(vm.ProductUUID))
				vmsForLink = append(vmsForLink, notifierPayload)
			}
			common.NotifyAdapters(runtime, dbc, vmsForLink...)
		}
	}
}

func handleNodeSignal(dbc db.DBSyncClient, ctx *sfPlugins.StatefunContextProcessor, payload *easyjson.JSON) {
	le := lg.GetLogger()
	logCtx := context.Background()

	nodeUID, ok := payload.GetByPath("UID").AsString()
	if !ok {
		le.Errorf(logCtx, "handleNodeSignal: cannot get UID from payload")
		return
	}

	systemUID, ok := payload.GetByPath("systemUID").AsString()
	if !ok {
		le.Errorf(logCtx, "handleNodeSignal: cannot get systemUID from payload")
		return
	}
	systemUID = strings.ToLower(systemUID)

	domain, ok := payload.GetByPath("domain").AsString()
	if !ok {
		le.Errorf(logCtx, "handleNodeSignal: cannot get domain from payload")
		return
	}

	// Find VMs with matching product_uuid
	vms, err := getVirtualMachines(dbc)
	if err != nil {
		le.Errorf(logCtx, "handleNodeSignal: cannot get virtual machines: %v", err)
		return
	}

	for _, vm := range vms {
		if vm.ProductUUID == systemUID {
			shadowID := ctx.Domain.CreateCustomShadowId(ctx.Domain.HubDomainName(), domain, ctx.Domain.GetObjectIDWithoutDomain(nodeUID))

			dbc.CMDB.ShadowObjectCanBeRecevier = true
			system.MsgOnErrorReturn(dbc.CMDB.ObjectCreate(shadowID, types.TYPE_FOLIAGE_NODE))
			dbc.CMDB.ShadowObjectCanBeRecevier = false

			system.MsgOnErrorReturn(dbc.CMDB.ObjectsLinkUpdate(vm.ID, shadowID, nil, easyjson.NewJSONObject(), false, shadowID))

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

		productUUID, ok := objData.GetByPath("body.sources.lshw.configuration.uuid").AsString()
		if ok {
			vms = append(vms, VirtualMachine{
				ID:          vmID,
				ProductUUID: productUUID,
			})
		}
	}
	return vms, nil
}
