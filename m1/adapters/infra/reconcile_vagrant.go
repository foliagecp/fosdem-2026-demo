package main

import (
	"fmt"
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

// reconcileVagrantGlobalStatus creates/updates VM objects based on raw `vagrant global-status` output.
//
// Expected JSON shape (simplified for demo):
//
//	{
//	  "metadata":{"machine_count":N},
//	  "vms":[{"id":"...","provider":"...","home":"...","name":"...","state":"..."}]
//	}
func reconcileVagrantGlobalStatus(dbc db.DBSyncClient, hypUUID, hostID string, raw easyjson.JSON, dm sfPlugins.Domain) {
	var arr []interface{}
	if raw.IsObject() {
		if a, ok := raw.GetByPath("vms").AsArray(); ok {
			arr = a
		} else if a, ok := raw.GetByPath("data.vms").AsArray(); ok {
			arr = a
		}
	} else {
		if a, ok := raw.AsArray(); ok {
			arr = a
		}
	}

	if arr == nil {
		lg.Logln(lg.WarnLevel, "infra: vagrant_global_status raw has no vms array")
		return
	}

	// Keep raw at hypervisor level for debugging.
	hupd := easyjson.NewJSONObject()
	hupd.SetByPath("sources.vagrant_global_status", raw)
	hupd.SetByPath("summary.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, hupd, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)

	desired := map[string]easyjson.JSON{}
	for _, item := range arr {
		vm := easyjson.NewJSON(item)
		vmHostID := strings.TrimSpace(vm.GetByPath("id").AsStringDefault(""))
		vmUUID := ensureVM(dbc, vmHostID)
		desired[vmUUID] = vm
	}

	// Delete stale VMs linked under this hypervisor.
	if uuids, err := dbc.Query.JPGQLCtraQuery(hypUUID, fmt.Sprintf(".*[l:type('%s')]", types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)); err == nil {
		for _, u := range uuids {
			uuid := dm.GetObjectIDWithoutDomain(u)
			if _, ok := desired[uuid]; !ok {
				_ = dbc.CMDB.ObjectDelete(uuid)
			}
		}
	}

	for vmUUID, vm := range desired {
		upd := easyjson.NewJSONObject()
		upd.SetByPath("sources.vagrant_global_status", vm)
		if name := strings.TrimSpace(vm.GetByPath("name").AsStringDefault("")); name != "" {
			upd.SetByPath("summary.name", easyjson.NewJSON(name))
		}
		if st := strings.TrimSpace(vm.GetByPath("state").AsStringDefault("")); st != "" {
			upd.SetByPath("summary.state", easyjson.NewJSON(st))
		}
		if id := strings.TrimSpace(vm.GetByPath("id").AsStringDefault("")); id != "" {
			upd.SetByPath("identifiers.vagrant_id", easyjson.NewJSON(id))
		}
		_ = dbc.CMDB.ObjectUpdate(vmUUID, upd, false, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)
		// Link hypervisor <-> VM.
		_ = dbc.CMDB.ObjectsLinkUpdate(hypUUID, vmUUID, common.ErrorRelyLinkTags, easyjson.NewJSONObject(), false, vmUUID)
		_ = dbc.CMDB.ObjectsLinkUpdate(vmUUID, hypUUID, common.ErrorPropagateLinkTags, easyjson.NewJSONObject(), false, "hypervisor")
	}

	count := len(desired)
	c := easyjson.NewJSONObject()
	c.SetByPath("summary.vm_count", easyjson.NewJSON(count))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, c, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}
