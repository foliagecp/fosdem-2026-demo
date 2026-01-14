package types

const (
	// Foliage Applications -------------------------------
	TYPE_FOLIAGE_APP_CMD       = "foliage-app-cmd"
	TYPE_FOLIAGE_APP_CONNECTOR = "foliage-app-connector"
	TYPE_FOLIAGE_APP_ADAPTER   = "foliage-app-adapter"
	// ----------------------------------------------------

	// Connector sources (by command / data source) -------
	TYPE_FOLIAGE_CONNECTOR_LSMOD                 = "foliage-connector-lsmod"
	TYPE_FOLIAGE_CONNECTOR_VAGRANT_GLOBAL_STATUS = "foliage-connector-vagrant_global_status"
	TYPE_FOLIAGE_CONNECTOR_LSHW                  = "foliage-connector-lshw"
	// ----------------------------------------------------

	// Adapter (digital twin) -----------------------------
	TYPE_FOLIAGE_ADAPTER_INFRA           = "foliage-adapter-infra"
	TYPE_FOLIAGE_ADAPTER_SERVER          = "foliage-adapter-server"
	TYPE_FOLIAGE_ADAPTER_HYPERVISOR      = "foliage-adapter-hypervisor"
	TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE = "foliage-adapter-virtual_machine"

	// Component types derived from lshw ------------------
	TYPE_FOLIAGE_ADAPTER_CPU             = "foliage-adapter-cpu"
	TYPE_FOLIAGE_ADAPTER_RAM_STICK       = "foliage-adapter-ram_stick"
	TYPE_FOLIAGE_ADAPTER_DISK            = "foliage-adapter-disk"
	TYPE_FOLIAGE_ADAPTER_BIOS            = "foliage-adapter-bios"
	TYPE_FOLIAGE_ADAPTER_SN              = "foliage-adapter-sn"
	TYPE_FOLIAGE_ADAPTER_NETWORK_ADAPTER = "foliage-adapter-network_adapter"
	// ----------------------------------------------------

	// Cmd types ------------------------------------------
	TYPE_FOLIAGE_CMD_ACTION_RESULT = "foliage-cmd-action_result"
	// ----------------------------------------------------
)
