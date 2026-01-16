package types

const (
	// Foliage Applications -------------------------------
	TYPE_FOLIAGE_APP_CMD               = "foliage-app-cmd"
	TYPE_FOLIAGE_APP_CONNECTOR         = "foliage-app-connector"
	TYPE_FOLIAGE_APP_ADAPTER           = "foliage-app-adapter"
	TYPE_FOLIAGE_APP_CONNECTOR_ADAPTER = "foliage-app-connector-adapter"
	// ----------------------------------------------------

	// Foliage Connector-Adapter --------------------------
	TYPE_FOLIAGE_K8S_INFRASTRUCTURE = "foliage-cn_ad-k8s-infrastructure"
	TYPE_FOLIAGE_CLUSTER            = "foliage-cn_ad-k8s-cluster"
	TYPE_FOLIAGE_NODE               = "foliage-cn_ad-k8s-node"
	TYPE_FOLIAGE_POD                = "foliage-cn_ad-k8s-pod"
	TYPE_FOLIAGE_DEPLOYMENT         = "foliage-cn_ad-k8s-deployment"
	TYPE_FOLIAGE_REPLICATION_SET    = "foliage-cn_ad-k8s-replication_set"
	// ----------------------------------------------------
)

// tags
const (
	TAG_SOURCE_TYPE_POD             = "k8s-source-pod"
	TAG_SOURCE_TYPE_NODE            = "k8s-source-node"
	TAG_SOURCE_TYPE_REPLICATION_SET = "k8s-source-replication_set"
	TAG_SOURCE_TYPE_DEPLOYMENT      = "k8s-source-deployment"
	TAG_SOURCE_TYPE_CLUSTER         = "k8s-source-cluster"
)
