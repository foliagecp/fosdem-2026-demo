package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
)

// lsmodDetectKVM parses the normalized JSON produced by the agent for `lsmod`
// and returns whether KVM-related modules are present.
func lsmodDetectKVM(raw easyjson.JSON) (kvmLoaded bool, modules []string) {
	modules = []string{}
	if raw.IsObject() {
		if a, ok := raw.GetByPath("modules").AsArray(); ok {
			for _, it := range a {
				j := easyjson.NewJSON(it)
				if j.IsString() {
					modules = append(modules, j.AsStringDefault(""))
					continue
				}
				modules = append(modules, j.GetByPath("name").AsStringDefault(""))
			}
		}
	}

	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		// KVM is typically represented as kvm + kvm_intel/kvm_amd.
		if m == "kvm" || strings.HasPrefix(m, "kvm_") {
			kvmLoaded = true
			break
		}
	}
	return kvmLoaded, modules
}

// reconcileLsmod updates hypervisor state based on raw `lsmod` output.
//
// Example expected JSON:
//
//	{"modules":[{"name":"kvm"},{"name":"kvm_intel"}]}
//	{"modules":["kvm","kvm_intel"]}
func reconcileLsmod(dbc db.DBSyncClient, hypUUID string, raw easyjson.JSON) {
	kvmLoaded, mods := lsmodDetectKVM(raw)

	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lsmod", raw)
	data.SetByPath("summary.kvm_module_loaded", easyjson.NewJSON(kvmLoaded))
	data.SetByPath("summary.modules", easyjson.NewJSON(mods))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
}
