package common

import (
	"fmt"
	"strings"

	"github.com/foliagecp/easyjson"
	"github.com/foliagecp/sdk/clients/go/db"
	lg "github.com/foliagecp/sdk/statefun/logger"
	sfPlugins "github.com/foliagecp/sdk/statefun/plugins"
	"github.com/foliagecp/sdk/statefun/system"
)

var ErrorPropagateLinkTags = []string{"__error_propagate"}

const PropagateErrorFunctionName = "functions.common.propagate_error"

// "error" bool
// "error_distribution" bool
// "__error_timestamp_nano" int

func PropagateError(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	var errorTS int64
	errorTSFloat, ok := ctx.Payload.GetByPath("body.__error_timestamp_nano").AsNumeric()
	if !ok {
		errorTS = system.GetCurrentTimeNs()
	} else {
		errorTS = int64(errorTSFloat)
	}

	dbc, err := db.NewDBSyncClientFromRequestFunction(ctx.Request)
	if err != nil {
		lg.Logf(lg.ErrorLevel, "PropagateError: cant create db sync client: %v", err)
		return
	}

	currentObject, err := dbc.CMDB.ObjectRead(ctx.Self.ID)
	if err != nil {
		lg.Logf(lg.ErrorLevel, "PropagateError: cant read object: %s", err)
		return
	}

	if currentObject.PathExists("body.error.error") || currentObject.PathExists("body.error.error_distribution") {
		existingTSFloat := currentObject.GetByPath("body.error.__error_timestamp_nano").AsNumericDefault(0)
		if int64(existingTSFloat) >= errorTS {
			lg.Logf(lg.DebugLevel, "PropagateError: existing error on object %s newer than propagating (%.0f > %d)", ctx.Self.ID, existingTSFloat, errorTS)
			return
		}
	}

	propagateErrorPayload := easyjson.NewJSONObject()
	propagateErrorPayload.SetByPath("error.distribution", easyjson.NewJSON(true))
	propagateErrorPayload.SetByPath("error.__error_timestamp_nano", easyjson.NewJSON(errorTS))
	if err = dbc.CMDB.ObjectUpdate(ctx.Self.ID, propagateErrorPayload, false); err != nil {
		lg.Logf(lg.ErrorLevel, "PropagateError: cant update object %s: %v", ctx.Self.ID, err)
	}

	for _, tag := range ErrorPropagateLinkTags {
		query := fmt.Sprintf(".*[l:tags('%s')]", tag)
		linkedIDs, err := dbc.Query.JPGQLCtraQuery(ctx.Self.ID, query)
		lg.Logf(lg.DebugLevel, "PropagateError: linkedIDs for %s: %v", ctx.Self.ID, linkedIDs)
		if err == nil && len(linkedIDs) > 0 {
			propagateToLinked(ctx, linkedIDs)
			return
		}
	}
}

func propagateToLinked(ctx *sfPlugins.StatefunContextProcessor, linkedIDs []string) {
	for _, linkedID := range linkedIDs {
		if linkedID == ctx.Self.ID || strings.HasSuffix(linkedID, ctx.Self.ID) {
			continue
		}

		system.MsgOnErrorReturn(ctx.Signal(
			sfPlugins.AutoSignalSelect,
			PropagateErrorFunctionName,
			ctx.Domain.GetObjectIDByShadowObjectID(linkedID),
			nil,
			nil,
		))
	}
}
