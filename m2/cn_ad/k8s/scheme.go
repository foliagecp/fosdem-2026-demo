package main

import (
	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/fosdem-2026-demo/m2"
	"github.com/foliagecp/sdk/clients/go/db"
	"github.com/foliagecp/sdk/statefun/system"
)

func createScheme(dbc db.DBSyncClient) {
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.CONNECTOR_ADAPTER_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.INFRASTRUCTURE_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.CLUSTER_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.NODE_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.POD_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.DEPLOYMENT_TYPE, easyjson.NewJSONObject(), false, true))
	system.MsgOnErrorReturn(dbc.CMDB.TypeUpdate(m2.REPLICATION_SET_TYPE, easyjson.NewJSONObject(), false, true))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CONNECTOR_ADAPTER_TYPE, m2.INFRASTRUCTURE_TYPE, nil, easyjson.NewJSONObject(), false, m2.INFRASTRUCTURE_TYPE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.INFRASTRUCTURE_TYPE, m2.CLUSTER_TYPE, nil, easyjson.NewJSONObject(), false, m2.CLUSTER_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.INFRASTRUCTURE_TYPE, m2.NODE_TYPE, nil, easyjson.NewJSONObject(), false, m2.NODE_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.INFRASTRUCTURE_TYPE, m2.POD_TYPE, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.INFRASTRUCTURE_TYPE, m2.DEPLOYMENT_TYPE, nil, easyjson.NewJSONObject(), false, m2.DEPLOYMENT_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.INFRASTRUCTURE_TYPE, m2.REPLICATION_SET_TYPE, nil, easyjson.NewJSONObject(), false, m2.REPLICATION_SET_TYPE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.NODE_TYPE, nil, easyjson.NewJSONObject(), false, m2.NODE_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.POD_TYPE, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.DEPLOYMENT_TYPE, nil, easyjson.NewJSONObject(), false, m2.DEPLOYMENT_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.CLUSTER_TYPE, m2.REPLICATION_SET_TYPE, nil, easyjson.NewJSONObject(), false, m2.REPLICATION_SET_TYPE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.NODE_TYPE, m2.POD_TYPE, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.POD_TYPE, m2.NODE_TYPE, nil, easyjson.NewJSONObject(), false, m2.NODE_TYPE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.DEPLOYMENT_TYPE, m2.POD_TYPE, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.POD_TYPE, m2.DEPLOYMENT_TYPE, nil, easyjson.NewJSONObject(), false, m2.DEPLOYMENT_TYPE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.DEPLOYMENT_TYPE, m2.REPLICATION_SET_TYPE, nil, easyjson.NewJSONObject(), false, m2.REPLICATION_SET_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.REPLICATION_SET_TYPE, m2.DEPLOYMENT_TYPE, nil, easyjson.NewJSONObject(), false, m2.DEPLOYMENT_TYPE))

	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.REPLICATION_SET_TYPE, m2.POD_TYPE, nil, easyjson.NewJSONObject(), false, m2.POD_TYPE))
	system.MsgOnErrorReturn(dbc.CMDB.TypesLinkUpdate(m2.POD_TYPE, m2.REPLICATION_SET_TYPE, nil, easyjson.NewJSONObject(), false, m2.REPLICATION_SET_TYPE))
}
