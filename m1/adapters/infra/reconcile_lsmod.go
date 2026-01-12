package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
)

// reconcileLsmod updates hypervisor state based on raw `lsmod` output.
//
// Example expected JSON:
//   {"modules":[{"name":"kvm"},{"name":"kvm_intel"}]}
//   {"modules":["kvm","kvm_intel"]}
func reconcileLsmod(dbc db.DBSyncClient, hypUUID string, raw easyjson.JSON) {
	mods := []string{}
	if raw.IsObject() {
		if a, ok := raw.GetByPath("modules").AsArray(); ok {
			for _, it := range a {
				j := easyjson.NewJSON(it)
				if j.IsString() {
					mods = append(mods, j.AsStringDefault(""))
					continue
				}
				mods = append(mods, j.GetByPath("name").AsStringDefault(""))
			}
		}
	}

	kvmLoaded := false
	for _, m := range mods {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		// KVM is typically represented as kvm + kvm_intel/kvm_amd.
		if m == "kvm" || strings.HasPrefix(m, "kvm_") {
			kvmLoaded = true
		}
	}

	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lsmod", raw)
	data.SetByPath("summary.kvm_module_loaded", easyjson.NewJSON(kvmLoaded))
	data.SetByPath("summary.modules", easyjson.NewJSON(mods))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}
