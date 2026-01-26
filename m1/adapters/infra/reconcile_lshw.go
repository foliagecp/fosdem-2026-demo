package main

import (
	"fmt"
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
)

// reconcileLshwServer updates the Server object and its derived component objects from raw `lshw -json`.
func reconcileLshwServer(dbc db.DBSyncClient, serverUUID string, raw easyjson.JSON) {
	reconcileLshwGeneric(dbc, serverUUID, types.TYPE_FOLIAGE_ADAPTER_SERVER, raw)
}

// reconcileLshwVM updates the Virtual Machine object and its derived component objects from raw `lshw -json`.
func reconcileLshwVM(dbc db.DBSyncClient, vmUUID string, raw easyjson.JSON) {
	reconcileLshwGeneric(dbc, vmUUID, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, raw)
}

func reconcileLshwGeneric(dbc db.DBSyncClient, parentUUID, parentType string, raw easyjson.JSON) {
	// Persist raw snapshot for debugging.
	upd := easyjson.NewJSONObject()
	upd.SetByPath("sources.lshw", raw)

	// Best-effort hostname / system id.
	if id := strings.TrimSpace(raw.GetByPath("id").AsStringDefault("")); id != "" {
		upd.SetByPath("summary.hostname", easyjson.NewJSON(id))
	}
	if prod := strings.TrimSpace(raw.GetByPath("product").AsStringDefault("")); prod != "" {
		upd.SetByPath("summary.product", easyjson.NewJSON(prod))
	}
	if vendor := strings.TrimSpace(raw.GetByPath("vendor").AsStringDefault("")); vendor != "" {
		upd.SetByPath("summary.vendor", easyjson.NewJSON(vendor))
	}
	// Extract configuration.uuid (product_uuid) for VM linking with K8s Nodes
	if cfgUUID := strings.TrimSpace(raw.GetByPath("configuration.uuid").AsStringDefault("")); cfgUUID != "" {
		upd.SetByPath("uuid", easyjson.NewJSON(strings.ToLower(cfgUUID)))
	}
	if sn := strings.TrimSpace(firstStringByKey(raw, "serial")); sn != "" {
		upd.SetByPath("summary.serial", easyjson.NewJSON(sn))
		ensureSNObject(dbc, parentUUID, sn)
	}

	// IP address can be present in a network node configuration.
	if ip := strings.TrimSpace(firstIP(raw)); ip != "" {
		upd.SetByPath("summary.detected_ip", easyjson.NewJSON(ip))
	}

	_ = dbc.CMDB.ObjectUpdate(parentUUID, upd, false, parentType)

	// CPU (first processor node).
	if cpu := findFirstNode(raw, func(n easyjson.JSON) bool {
		return strings.TrimSpace(n.GetByPath("class").AsStringDefault("")) == "processor"
	}); cpu.IsObject() {
		ensureCPUObject(dbc, parentUUID, cpu)
		// Virtualization flags.
		vmx, svm := cpuHasVirt(cpu)
		vupd := easyjson.NewJSONObject()
		vupd.SetByPath("summary.virtualization.vmx", easyjson.NewJSON(vmx))
		vupd.SetByPath("summary.virtualization.svm", easyjson.NewJSON(svm))
		vupd.SetByPath("summary.server_virtualization_technology", easyjson.NewJSON(typeOfVirtualization(vmx, svm)))
		_ = dbc.CMDB.ObjectUpdate(parentUUID, vupd, false, parentType)
	}

	// BIOS / firmware.
	if fw := findFirstNode(raw, func(n easyjson.JSON) bool {
		if strings.TrimSpace(n.GetByPath("id").AsStringDefault("")) == "firmware" {
			return true
		}
		desc := strings.ToLower(strings.TrimSpace(n.GetByPath("description").AsStringDefault("")))
		return strings.Contains(desc, "bios") || strings.Contains(desc, "firmware")
	}); fw.IsObject() {
		ensureBIOSObject(dbc, parentUUID, fw)
	}

	// Disks.
	disks := findAllNodes(raw, func(n easyjson.JSON) bool {
		return strings.TrimSpace(n.GetByPath("class").AsStringDefault("")) == "disk"
	})
	for idx, d := range disks {
		ensureDiskObject(dbc, parentUUID, idx, d)
	}

	// RAM sticks (best-effort: memory nodes with a size).
	rams := findAllNodes(raw, func(n easyjson.JSON) bool {
		if strings.TrimSpace(n.GetByPath("class").AsStringDefault("")) != "memory" {
			return false
		}
		// Prefer DIMM-like banks.
		id := strings.ToLower(strings.TrimSpace(n.GetByPath("id").AsStringDefault("")))
		if strings.HasPrefix(id, "bank") {
			return true
		}
		// Or a node that has size (may be system memory or DIMM).
		return n.GetByPath("size").IsNumeric()
	})
	for idx, r := range rams {
		ensureRAMObject(dbc, parentUUID, idx, r)
	}

	// Network adapters.
	nics := findAllNodes(raw, func(n easyjson.JSON) bool {
		return strings.TrimSpace(n.GetByPath("class").AsStringDefault("")) == "network"
	})
	for idx, nic := range nics {
		ensureNICObject(dbc, parentUUID, idx, nic)
	}
}

func ensureSNObject(dbc db.DBSyncClient, parentUUID, serial string) {
	uuid := getSerialNumberUUID(parentUUID)
	data := easyjson.NewJSONObject()
	data.SetByPath("summary.serial", easyjson.NewJSON(serial))
	_ = dbc.CMDB.ObjectUpdate(uuid, data, false, types.TYPE_FOLIAGE_ADAPTER_SN)
	_ = dbc.CMDB.ObjectsLinkUpdate(parentUUID, uuid, nil, easyjson.NewJSONObject(), false, "sn")
}

func ensureCPUObject(dbc db.DBSyncClient, parentUUID string, cpu easyjson.JSON) {
	uuid := getCpuUUID(parentUUID)
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lshw", cpu)
	if vendor := strings.TrimSpace(cpu.GetByPath("vendor").AsStringDefault("")); vendor != "" {
		data.SetByPath("summary.vendor", easyjson.NewJSON(vendor))
	}
	if prod := strings.TrimSpace(cpu.GetByPath("product").AsStringDefault("")); prod != "" {
		data.SetByPath("summary.product", easyjson.NewJSON(prod))
	}
	if cores := cpu.GetByPath("configuration.cores").AsNumericDefault(0); cores > 0 {
		data.SetByPath("summary.cores", easyjson.NewJSON(int64(cores)))
	}
	if enabled := cpu.GetByPath("configuration.enabledcores").AsNumericDefault(0); enabled > 0 {
		data.SetByPath("summary.enabled_cores", easyjson.NewJSON(int64(enabled)))
	}
	if cap, ok := cpu.GetByPath("capabilities").AsArray(); ok {
		caps := []string{}
		for _, it := range cap {
			j := easyjson.NewJSON(it)
			if s, ok := j.AsString(); ok {
				caps = append(caps, s)
			}
		}
		data.SetByPath("summary.capabilities", easyjson.NewJSON(caps))
	}
	_ = dbc.CMDB.ObjectUpdate(uuid, data, false, types.TYPE_FOLIAGE_ADAPTER_CPU)
	_ = dbc.CMDB.ObjectsLinkUpdate(parentUUID, uuid, nil, easyjson.NewJSONObject(), false, "cpu")
}

func ensureBIOSObject(dbc db.DBSyncClient, parentUUID string, fw easyjson.JSON) {
	uuid := getBiosUUID(parentUUID)
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lshw", fw)
	if vendor := strings.TrimSpace(fw.GetByPath("vendor").AsStringDefault("")); vendor != "" {
		data.SetByPath("summary.vendor", easyjson.NewJSON(vendor))
	}
	if ver := strings.TrimSpace(fw.GetByPath("version").AsStringDefault("")); ver != "" {
		data.SetByPath("summary.version", easyjson.NewJSON(ver))
	}
	if date := strings.TrimSpace(fw.GetByPath("date").AsStringDefault("")); date != "" {
		data.SetByPath("summary.date", easyjson.NewJSON(date))
	}
	_ = dbc.CMDB.ObjectUpdate(uuid, data, false, types.TYPE_FOLIAGE_ADAPTER_BIOS)
	_ = dbc.CMDB.ObjectsLinkUpdate(parentUUID, uuid, nil, easyjson.NewJSONObject(), false, "bios")
}

func ensureDiskObject(dbc db.DBSyncClient, parentUUID string, idx int, d easyjson.JSON) {
	uuid := getDiskUUID(parentUUID, idx)
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lshw", d)
	if prod := strings.TrimSpace(d.GetByPath("product").AsStringDefault("")); prod != "" {
		data.SetByPath("summary.product", easyjson.NewJSON(prod))
	}
	if vendor := strings.TrimSpace(d.GetByPath("vendor").AsStringDefault("")); vendor != "" {
		data.SetByPath("summary.vendor", easyjson.NewJSON(vendor))
	}
	if serial := strings.TrimSpace(firstStringByKey(d, "serial")); serial != "" {
		data.SetByPath("summary.serial", easyjson.NewJSON(serial))
	}
	if sz := d.GetByPath("size").AsNumericDefault(0); sz > 0 {
		data.SetByPath("summary.size_bytes", easyjson.NewJSON(int64(sz)))
	}
	_ = dbc.CMDB.ObjectUpdate(uuid, data, false, types.TYPE_FOLIAGE_ADAPTER_DISK)
	_ = dbc.CMDB.ObjectsLinkUpdate(parentUUID, uuid, nil, easyjson.NewJSONObject(), false, fmt.Sprintf("disk_%d", idx))
}

func ensureRAMObject(dbc db.DBSyncClient, parentUUID string, idx int, r easyjson.JSON) {
	uuid := getRamUUID(parentUUID, idx)
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lshw", r)
	if prod := strings.TrimSpace(r.GetByPath("product").AsStringDefault("")); prod != "" {
		data.SetByPath("summary.product", easyjson.NewJSON(prod))
	}
	if vendor := strings.TrimSpace(r.GetByPath("vendor").AsStringDefault("")); vendor != "" {
		data.SetByPath("summary.vendor", easyjson.NewJSON(vendor))
	}
	if serial := strings.TrimSpace(firstStringByKey(r, "serial")); serial != "" {
		data.SetByPath("summary.serial", easyjson.NewJSON(serial))
	}
	if sz := r.GetByPath("size").AsNumericDefault(0); sz > 0 {
		data.SetByPath("summary.size_bytes", easyjson.NewJSON(int64(sz)))
	}
	_ = dbc.CMDB.ObjectUpdate(uuid, data, false, types.TYPE_FOLIAGE_ADAPTER_RAM_STICK)
	_ = dbc.CMDB.ObjectsLinkUpdate(parentUUID, uuid, nil, easyjson.NewJSONObject(), false, fmt.Sprintf("ram_%d", idx))
}

func ensureNICObject(dbc db.DBSyncClient, parentUUID string, idx int, nic easyjson.JSON) {
	uuid := getNicUUID(parentUUID, idx)
	data := easyjson.NewJSONObject()
	data.SetByPath("sources.lshw", nic)
	if prod := strings.TrimSpace(nic.GetByPath("product").AsStringDefault("")); prod != "" {
		data.SetByPath("summary.product", easyjson.NewJSON(prod))
	}
	if vendor := strings.TrimSpace(nic.GetByPath("vendor").AsStringDefault("")); vendor != "" {
		data.SetByPath("summary.vendor", easyjson.NewJSON(vendor))
	}
	if mac := strings.TrimSpace(nic.GetByPath("serial").AsStringDefault("")); mac != "" {
		data.SetByPath("summary.mac", easyjson.NewJSON(mac))
	}
	if ln := nic.GetByPath("logicalname"); ln.IsString() {
		data.SetByPath("summary.logicalname", ln)
	}
	if ip := nic.GetByPath("configuration.ip"); ip.IsString() {
		data.SetByPath("summary.ip", ip)
	}
	_ = dbc.CMDB.ObjectUpdate(uuid, data, false, types.TYPE_FOLIAGE_ADAPTER_NETWORK_ADAPTER)
	_ = dbc.CMDB.ObjectsLinkUpdate(parentUUID, uuid, nil, easyjson.NewJSONObject(), false, fmt.Sprintf("nic_%d", idx))
}
