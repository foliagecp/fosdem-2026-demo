package main

import (
	"strings"

	easyjson "github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/common"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/apps"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/types"
	"github.com/foliagecp/fosdem-2026-demo/m1/common/util"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
)

// Command identifiers used across agents, connectors and this adapter.
const (
	cmdLsmod               = "lsmod"
	cmdVagrantGlobalStatus = "vagrant_global_status"
	cmdLshw                = "lshw"
)

// infraPushUpdate is the single entry point for rebuilding the infrastructure digital twin.
// It is triggered by connectors via ctx.Signal(...) after they ingest a new raw source snapshot.
func infraPushUpdate(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logln(lg.ErrorLevel, "infra.push_update: cannot create db client")
		return
	}

	// Filter updates by the connector that emitted the signal.
	callerID := strings.TrimSpace(ctx.Caller.ID)
	callerShort := ctx.Domain.GetObjectIDWithoutDomain(callerID)
	switch callerShort {
	case apps.APP_CN_SERVER, apps.APP_CN_HYPERVISOR, apps.APP_CN_VIRTUAL_MACHINE:
		// ok
	default:
		lg.Logf(lg.DebugLevel, "infra.push_update: ignoring signal from caller=%s", callerID)
		return
	}

	payload := ctx.Payload.NormalizedClone()

	hostID, err := mustString(payload, "host_id")
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: %v", err)
		return
	}
	command, err := mustString(payload, "command")
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: %v", err)
		return
	}
	sourceUUID, err := mustString(payload, "source_uuid")
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: %v", err)
		return
	}
	sourceType := payload.GetByPath("source_type").AsStringDefault("")

	srcObj, err := dbc.CMDB.ObjectRead(sourceUUID)
	if err != nil {
		lg.Logf(lg.ErrorLevel, "infra.push_update: cannot read source %s: %v", sourceUUID, err)
		return
	}
	raw := srcObj.GetByPath("body").NormalizedClone()

	// Route by caller + command.
	switch callerShort {
	case apps.APP_CN_SERVER:
		switch command {
		case cmdLshw:
			serverUUID := ensureServer(dbc, hostID)
			reconcileLshwServer(dbc, serverUUID, raw)
		default:
			lg.Logf(lg.WarnLevel, "infra.push_update: unknown server command=%s source_type=%s", command, sourceType)
		}

	case apps.APP_CN_HYPERVISOR:
		switch command {
		case cmdLsmod:
			hasKvm, _ := lsmodDetectKVM(raw)
			if !hasKvm {
				lg.Logf(lg.InfoLevel, "infra.push_update: skipping hypervisor (no kvm in lsmod) host_id=%s", hostID)
				break
			}
			serverUUID := ensureServer(dbc, hostID)
			hypUUID := ensureHypervisor(dbc, serverUUID, hostID)
			reconcileLsmod(dbc, hypUUID, raw)
		default:
			lg.Logf(lg.WarnLevel, "infra.push_update: unknown hypervisor command=%s source_type=%s", command, sourceType)
		}

	case apps.APP_CN_VIRTUAL_MACHINE:
		switch command {
		case cmdVagrantGlobalStatus:
			// Only consider Vagrant VMs for KVM hosts. The hypervisor object must be created
			// by a prior `lsmod` ingestion that detected KVM.
			probeHypUUID := getHypervisorUUID(hostID)
			if _, err := dbc.CMDB.ObjectRead(probeHypUUID); err != nil {
				lg.Logf(lg.InfoLevel, "infra.push_update: skipping vagrant_global_status (no kvm hypervisor detected yet) host_id=%s", hostID)
				break
			}
			serverUUID := ensureServer(dbc, hostID)
			hypUUID := ensureHypervisor(dbc, serverUUID, hostID)
			reconcileVagrantGlobalStatus(dbc, hypUUID, hostID, raw, ctx.Domain)
		case cmdLshw:
			vmUUID := ensureVM(dbc, hostID)
			reconcileLshwVM(dbc, vmUUID, raw)
		default:
			lg.Logf(lg.WarnLevel, "infra.push_update: unknown vm command=%s source_type=%s", command, sourceType)
		}
	}

	notifierPayload := easyjson.NewJSONObject()
	notifierPayload.SetByPath("domain", easyjson.NewJSON(ctx.Domain.Name()))
	notifierPayload.SetByPath("id", easyjson.NewJSON(infraRootUUID))
	notifierPayload.SetByPath("type", easyjson.NewJSON(types.TYPE_FOLIAGE_ADAPTER_INFRA))
	notifierPayload.SetByPath("operation", easyjson.NewJSON("link_model"))
	common.PostProcessNotifier(dbc, ctx, notifierPayload.GetPtr())

	adapterUpdateStatus(dbc)
}

func ensureInfraLink(dbc db.DBSyncClient, childUUID, linkName string) {
	_ = dbc.CMDB.ObjectsLinkUpdate(infraRootUUID, childUUID, nil, easyjson.NewJSONObject(), false, linkName)
}

// ensureServer creates/updates the server node for a given host_id.
// host_id is derived from IP and is used as the server UUID to keep object ids readable.
func ensureServer(dbc db.DBSyncClient, hostID string) string {
	hostID = util.GetSafeName(hostID)
	serverUUID := hostID
	if serverUUID == "" {
		serverUUID = "unknown_host"
	}
	data := easyjson.NewJSONObject()
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(serverUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_SERVER)
	ensureInfraLink(dbc, serverUUID, hostID)
	return serverUUID
}

// ensureHypervisor creates/updates a hypervisor node for a given server.
func ensureHypervisor(dbc db.DBSyncClient, serverUUID, hostID string) string {
	hostID = util.GetSafeName(hostID)
	hypUUID := getHypervisorUUID(hostID)
	data := easyjson.NewJSONObject()
	data.SetByPath("summary.kind", easyjson.NewJSON("KVM"))
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(hypUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_HYPERVISOR)
	// infra -> hypervisor
	ensureInfraLink(dbc, hypUUID, hypUUID)
	// server <-> hypervisor
	_ = dbc.CMDB.ObjectsLinkUpdate(serverUUID, hypUUID, nil, easyjson.NewJSONObject(), false, "kvm")
	_ = dbc.CMDB.ObjectsLinkUpdate(hypUUID, serverUUID, nil, easyjson.NewJSONObject(), false, "server")
	return hypUUID
}

// ensureVM creates/updates a VM node for a given host_id (derived from VM IP).
func ensureVM(dbc db.DBSyncClient, hostID string) string {
	hostID = util.GetSafeName(hostID)
	vmUUID := getVirtualMachineUUID(hostID)
	data := easyjson.NewJSONObject()
	data.SetByPath("identifiers.host_id", easyjson.NewJSON(hostID))
	_ = dbc.CMDB.ObjectUpdate(vmUUID, data, false, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE)
	ensureInfraLink(dbc, vmUUID, hostID)
	return vmUUID
}
