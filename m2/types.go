package m2

const (
	CLUSTER_TYPE         = "foliage-k8s-cluster"
	NODE_TYPE            = "foliage-k8s-node"
	POD_TYPE             = "foliage-k8s-pod"
	DEPLOYMENT_TYPE      = "foliage-k8s-deployment"
	REPLICATION_SET_TYPE = "foliage-k8s-replication_set"

	K8S_INFORMER_TYPE = "foliage-k8s-informer"

	CONNECTOR_TYPE = "foliage_connectors_connector"
	ADAPTER_TYPE   = "foliage_adapters_adapter"
)

// tags
const (
	TAG_SOURCE_TYPE_POD             = "k8s-source-pod"
	TAG_SOURCE_TYPE_NODE            = "k8s-source-node"
	TAG_SOURCE_TYPE_REPLICATION_SET = "k8s-source-replication_set"
	TAG_SOURCE_TYPE_DEPLOYMENT      = "k8s-source-deployment"
	TAG_SOURCE_TYPE_CLUSTER         = "k8s-source-cluster"
)

const DOMAIN_NAME = "m2"
