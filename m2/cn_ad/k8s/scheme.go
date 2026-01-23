package main

import (
	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2/common/types"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun/system"
)

func createScheme(dbc db.DBSyncClient) {
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_CLUSTER, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_NODE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_POD, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_DEPLOYMENT, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_REPLICATION_SET, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_APP_ADAPTER, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, types.TYPE_FOLIAGE_CLUSTER, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_CLUSTER))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, types.TYPE_FOLIAGE_NODE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_NODE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, types.TYPE_FOLIAGE_POD, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_POD))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, types.TYPE_FOLIAGE_DEPLOYMENT, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_DEPLOYMENT))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_K8S_INFRASTRUCTURE, types.TYPE_FOLIAGE_REPLICATION_SET, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_REPLICATION_SET))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_CLUSTER, types.TYPE_FOLIAGE_NODE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_NODE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_CLUSTER, types.TYPE_FOLIAGE_POD, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_POD))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_CLUSTER, types.TYPE_FOLIAGE_DEPLOYMENT, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_DEPLOYMENT))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_CLUSTER, types.TYPE_FOLIAGE_REPLICATION_SET, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_REPLICATION_SET))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_NODE, types.TYPE_FOLIAGE_POD, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_POD))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_POD, types.TYPE_FOLIAGE_NODE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_NODE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_DEPLOYMENT, types.TYPE_FOLIAGE_POD, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_POD))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_POD, types.TYPE_FOLIAGE_DEPLOYMENT, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_DEPLOYMENT))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_DEPLOYMENT, types.TYPE_FOLIAGE_REPLICATION_SET, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_REPLICATION_SET))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_REPLICATION_SET, types.TYPE_FOLIAGE_DEPLOYMENT, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_DEPLOYMENT))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_REPLICATION_SET, types.TYPE_FOLIAGE_POD, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_POD))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_POD, types.TYPE_FOLIAGE_REPLICATION_SET, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_REPLICATION_SET))

	// Architecture linking: Pod ↔ ArchBlock, Deployment ↔ ArchBlock
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_POD, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_DEPLOYMENT, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_ADAPTER_ARCH_BLOCK, types.TYPE_FOLIAGE_DEPLOYMENT, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_DEPLOYMENT))

	// Infrastructure linking: Node ↔ VM (shadow objects from M1)
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(types.TYPE_FOLIAGE_NODE, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE, nil, easyjson.NewJSONObject(), false, types.TYPE_FOLIAGE_ADAPTER_VIRTUAL_MACHINE))
}
