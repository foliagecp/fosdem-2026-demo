package common

import (
	"fmt"
	"slices"
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
	lg.Logf(lg.DebugLevel, "PropagateError() called on ID: %v", ctx.Self.ID)

	hasErrorField := ctx.Payload.PathExists("error")
	if !hasErrorField {
		lg.Logf(lg.DebugLevel,
			"PropagateError: no error field in payload for %s, skip",
			ctx.Self.ID,
		)
		return
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

	errorInPayload := ctx.Payload.GetByPath("error").AsBoolDefault(false)

	if errorInPayload {
		propagateErrorSet(ctx, dbc, currentObject)
	} else {
		propagateErrorClear(ctx, dbc, currentObject)
	}
}

func propagateErrorSet(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, currentObject easyjson.JSON) {
	var errorTS int64
	errorTSFloat, ok := ctx.Payload.GetByPath("__error_timestamp_nano").AsNumeric()
	if !ok {
		errorTS = system.GetCurrentTimeNs()
	} else {
		errorTS = int64(errorTSFloat)
	}

	isSourceError := ctx.Payload.GetByPath("error.error").AsBoolDefault(false)

	if currentObject.GetByPath("error.error").AsBoolDefault(false) || currentObject.GetByPathPtr("error.error_distribution").AsBoolDefault(false) {
		existingTSFloat := currentObject.GetByPath("error.__error_timestamp_nano").AsNumericDefault(0)
		if int64(existingTSFloat) >= errorTS {
			lg.Logf(lg.DebugLevel, "PropagateError: existing error on object %s newer than propagating (%.0f >= %d)", ctx.Self.ID, existingTSFloat, errorTS)
			return
		}
	}

	propagateErrorPayload := easyjson.NewJSONObject()
	if isSourceError {
		propagateErrorPayload.SetByPath("error.error", easyjson.NewJSON(true))
	} else {
		propagateErrorPayload.SetByPath("error.error_distribution", easyjson.NewJSON(true))
	}
	propagateErrorPayload.SetByPath("error.__error_timestamp_nano", easyjson.NewJSON(errorTS))

	if err := dbc.CMDB.ObjectUpdate(ctx.Self.ID, propagateErrorPayload, false); err != nil {
		lg.Logf(lg.ErrorLevel, "PropagateError: cant update object %s: %v", ctx.Self.ID, err)
	}

	lg.Logf(lg.DebugLevel, "PropagateError: marked object %s with error (source=%v, ts=%d)", ctx.Self.ID, isSourceError, errorTS)

	for _, tag := range ErrorPropagateLinkTags {
		query := fmt.Sprintf(".*[l:tags('%s')]", tag)
		linkedIDs, err := dbc.Query.JPGQLCtraQuery(ctx.Self.ID, query)
		lg.Logf(lg.DebugLevel, "PropagateError: outgoing linkedIDs for %s: %v", ctx.Self.ID, linkedIDs)
		if err == nil && len(linkedIDs) > 0 {
			propagateToLinked(ctx, linkedIDs, true, errorTS)
		}
	}
}

func propagateErrorClear(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, currentObject easyjson.JSON) {
	lg.Logf(lg.DebugLevel, "PropagateError: clearing error from object %s", ctx.Self.ID)

	hasError := currentObject.GetByPath("error.error").AsBoolDefault(false)
	hasErrorDistribution := currentObject.GetByPath("error.error_distribution").AsBoolDefault(false)

	if !hasError && !hasErrorDistribution {
		lg.Logf(lg.DebugLevel, "PropagateError: object %s has no error to clear", ctx.Self.ID)
		return
	}

	if !hasError {
		hasIncomingErrors, err := checkIncomingLinksForErrors(ctx, dbc)
		if err != nil {
			lg.Logf(lg.ErrorLevel, "PropagateError: error checking incoming links for %s: %v", ctx.Self.ID, err)
			return
		}

		if hasIncomingErrors {
			lg.Logf(lg.DebugLevel, "PropagateError: object %s still has incoming errors, not clearing", ctx.Self.ID)
			return
		}
	}

	clearPayload := easyjson.NewJSONObject()
	clearPayload.SetByPath("error", easyjson.NewJSON(nil))

	if err := dbc.CMDB.ObjectUpdate(ctx.Self.ID, clearPayload, false); err != nil {
		lg.Logf(lg.ErrorLevel, "PropagateError: cant clear error on object %s: %v", ctx.Self.ID, err)
		return
	}

	lg.Logf(lg.DebugLevel, "PropagateError: cleared error from object %s", ctx.Self.ID)

	for _, tag := range ErrorPropagateLinkTags {
		query := fmt.Sprintf(".*[l:tags('%s')]", tag)
		linkedIDs, err := dbc.Query.JPGQLCtraQuery(ctx.Self.ID, query)
		lg.Logf(lg.DebugLevel, "PropagateError: outgoing linkedIDs for clearing from %s: %v", ctx.Self.ID, linkedIDs)
		if err == nil && len(linkedIDs) > 0 {
			propagateToLinked(ctx, linkedIDs, false, 0)
		}
	}
}

func checkIncomingLinksForErrors(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient) (bool, error) {
	currentObject, err := dbc.CMDB.ObjectRead(ctx.Self.ID)
	if err != nil {
		return false, fmt.Errorf("cant read object: %w", err)
	}

	inLinks, ok := currentObject.GetByPath("to_objects").AsArrayString()
	if ok {
		for _, from := range inLinks {
			link, err := dbc.CMDB.ObjectsLinkRead(from, ctx.Self.ID)
			if err == nil {
				tags, ok := link.GetByPath("tags").AsArrayString()
				if ok {
					if len(ErrorPropagateLinkTags) > 0 && slices.Contains(tags, ErrorPropagateLinkTags[0]) {
						obj, err := dbc.CMDB.ObjectRead(ctx.Domain.GetObjectIDByShadowObjectID(from))
						if err == nil {
							if obj.GetByPath("error.error").AsBoolDefault(false) || obj.GetByPath("error.error_distribution").AsBoolDefault(false) {
								lg.Logf(lg.InfoLevel, "Object %s has incoming errors (from %s), not clearing", ctx.Self.ID, from)
								return true, nil
							}
						}
					}
				}
			}
		}
	}

	lg.Logf(lg.DebugLevel, "PropagateError: no incoming objects with errors for %s", ctx.Self.ID)
	return false, nil
}

func propagateToLinked(ctx *sfPlugins.StatefunContextProcessor, linkedIDs []string, setError bool, errorTS int64) {
	for _, linkedID := range linkedIDs {
		if linkedID == ctx.Self.ID || strings.HasSuffix(linkedID, ctx.Self.ID) {
			continue
		}

		var payload *easyjson.JSON
		if setError {
			p := easyjson.NewJSONObject()
			p.SetByPath("error.error_distribution", easyjson.NewJSON(true))
			p.SetByPath("error.__error_timestamp_nano", easyjson.NewJSON(errorTS))
			payload = &p
		} else {
			payload = nil
		}

		system.MsgOnErrorReturn(ctx.Signal(
			sfPlugins.AutoSignalSelect,
			PropagateErrorFunctionName,
			ctx.Domain.GetObjectIDByShadowObjectID(linkedID),
			payload,
			nil,
		))
	}
}
