package common

import (
	"fmt"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/sdk/clients/go/db"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var ErrorPropagateLinkTags = []string{"__error"}

const PropagateErrorFunctionName = "functions.common.propagate_error"

func PropagateError(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	errorMsg := ctx.Payload.GetByPath("error_msg").AsStringDefault("unknown error")
	errorTS, ok := ctx.Payload.GetByPath("error_time").AsNumeric()
	if !ok {
		errorTS = float64(system.GetCurrentTimeNs())
	}

	objectContext := ctx.GetObjectContext()
	if objectContext.PathExists("error") {
		existingTS := objectContext.GetByPath("error.timestamp").AsNumericDefault(0)
		if existingTS >= errorTS {
			return
		}
	}

	objectContext.SetByPath("error.timestamp", easyjson.NewJSON(errorTS))
	objectContext.SetByPath("error.message", easyjson.NewJSON(errorMsg))
	ctx.SetObjectContext(objectContext)

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		return
	}

	propagatePayload := easyjson.NewJSONObject()
	propagatePayload.SetByPath("error_msg", easyjson.NewJSON(errorMsg))
	propagatePayload.SetByPath("error_time", easyjson.NewJSON(errorTS))

	for _, tag := range ErrorPropagateLinkTags {
		query := fmt.Sprintf(".*[l:tag('%s')]", tag)
		linkedIDs, err := dbc.Query.JPGQLCtraQuery(ctx.Self.ID, query)
		if err == nil && len(linkedIDs) > 0 {
			propagateToLinked(ctx, linkedIDs, &propagatePayload)
			return
		}
	}
}

func propagateToLinked(ctx *sfPlugins.StatefunContextProcessor, linkedIDs []string, payload *easyjson.JSON) {
	for _, linkedID := range linkedIDs {
		if linkedID == ctx.Self.ID || strings.HasSuffix(linkedID, ctx.Self.ID) {
			continue
		}
		system.MsgOnErrorReturn(ctx.Signal(
			sfPlugins.AutoSignalSelect,
			PropagateErrorFunctionName,
			linkedID,
			payload,
			nil,
		))
	}
}
