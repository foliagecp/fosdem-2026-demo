package types

const (
	// Foliage Applications -------------------------------
	TYPE_FOLIAGE_APP_CMD       = "foliage-app-cmd"
	TYPE_FOLIAGE_APP_CONNECTOR = "foliage-app-connector"
	TYPE_FOLIAGE_APP_ADAPTER   = "foliage-app-adapter"
	// ----------------------------------------------------

	// Connector sources (by command) ---------------------
	TYPE_FOLIAGE_CONNECTOR_HOSTNAME           = "foliage-connector-hostname"
	TYPE_FOLIAGE_CONNECTOR_KVM_LIBVIRTD       = "foliage-connector-kvm_libvirtd"
	TYPE_FOLIAGE_CONNECTOR_KVM_VIRSH_LIST_ALL = "foliage-connector-kvm_virsh_list_all"
	// ----------------------------------------------------

	// Adapter (digital twin) -----------------------------
	TYPE_FOLIAGE_ADAPTER_INFRA           = "foliage-adapter-infra"
	TYPE_FOLIAGE_ADAPTER_SERVER          = "foliage-adapter-server"
	TYPE_FOLIAGE_ADAPTER_HYPERVISOR      = "foliage-adapter-hypervisor"
	TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE = "foliage-adapter-virtual_machine"
	// ----------------------------------------------------

	// Cmd types ------------------------------------------
	TYPE_FOLIAGE_CMD_ACTION_RESULT = "foliage-cmd-action_result"
	// ----------------------------------------------------
)
