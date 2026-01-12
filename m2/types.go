package m2

const (
	CONNECTOR_ADAPTER_TYPE = "foliage-k8s-cn_ad"
	INFRASTRUCTURE_TYPE    = "foliage-k8s-infrastructure"
	CLUSTER_TYPE           = "foliage-k8s-cluster"
	NODE_TYPE              = "foliage-k8s-node"
	POD_TYPE               = "foliage-k8s-pod"
	DEPLOYMENT_TYPE        = "foliage-k8s-deployment"
	REPLICATION_SET_TYPE   = "foliage-k8s-replication_set"

	K8S_INFORMER_TYPE = "foliage-k8s-informer"

	TYPE_FOLIAGE_APP_CONNECTOR = "foliage-app-connector"
	TYPE_FOLIAGE_APP_CMD       = "foliage-app-cmd"
	TYPE_FOLIAGE_APP_ADAPTER   = "foliage-app-adapter"
	APP_CMD                    = "cmd_m2"
)

// tags
const (
	TAG_SOURCE_TYPE_POD             = "k8s-source-pod"
	TAG_SOURCE_TYPE_NODE            = "k8s-source-node"
	TAG_SOURCE_TYPE_REPLICATION_SET = "k8s-source-replication_set"
	TAG_SOURCE_TYPE_DEPLOYMENT      = "k8s-source-deployment"
	TAG_SOURCE_TYPE_CLUSTER         = "k8s-source-cluster"
)
