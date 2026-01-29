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
var ErrorRelyLinkTags = []string{"__error_rely"}

const PropagateErrorFunctionName = "functions.common.propagate_error"

// "error" bool
// "error_distribution" bool
// "__error_timestamp_nano" int
// "__error_blast_radius" int

func PropagateError(_ sfPlugins.StatefunExecutor, ctx *sfPlugins.StatefunContextProcessor) {
	lg.Logf(lg.DebugLevel, "PropagateError() called on ID: %v", ctx.Self.ID)

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

	var errorTS int64
	errorTSFloat, ok := ctx.Payload.GetByPath("__error_timestamp_nano").AsNumeric()
	if !ok {
		errorTS = system.GetCurrentTimeNs()
	} else {
		errorTS = int64(errorTSFloat)
	}

	var blastRadius int
	blastRadiusFloat, ok := ctx.Payload.GetByPath("__error_blast_radius").AsNumeric()
	if !ok {
		blastRadius = 0
	} else {
		blastRadius = int(blastRadiusFloat)
	}

	if ctx.Payload.GetByPath("__delete").AsBoolDefault(false) {
		system.MsgOnErrorReturn(dbc.CMDB.ObjectDelete(ctx.Self.ID))
	}

	propagateToLinked(ctx, propagateErrorRecalculate(ctx, dbc, currentObject, errorTS, blastRadius), errorTS, blastRadius)
}

func propagateErrorRecalculate(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, currentObject easyjson.JSON, errorTS int64, blastRadius int) (continuePropagationIds []string) {
	existingTSFloat := currentObject.GetByPath("body.error.__error_timestamp_nano").AsNumericDefault(0)
	if int64(existingTSFloat) >= errorTS {
		lg.Logf(lg.DebugLevel, "PropagateError: existing error on object %s newer than propagating, prevent cycles (%.0f >= %d)", ctx.Self.ID, existingTSFloat, errorTS)
		return nil
	}

	existingBlastRadiusFloat := currentObject.GetByPath("body.error.__error_blast_radius").AsNumericDefault(9999)
	if int(existingBlastRadiusFloat) < blastRadius {
		lg.Logf(lg.DebugLevel, "PropagateError: existing blast_radius on object %s is less than propagating, has no effect (%.0f >= %d)", ctx.Self.ID, existingBlastRadiusFloat, blastRadius)
		return nil
	}

	propagateLinkedIDs := getLinked(ctx, dbc, ErrorPropagateLinkTags)
	relyLinkedIDs := getLinked(ctx, dbc, ErrorRelyLinkTags)

	propagateErrorPayload := easyjson.NewJSONObject()
	propagateErrorPayload.SetByPath("error.__error_timestamp_nano", easyjson.NewJSON(errorTS))
	propagateErrorPayload.SetByPath("error.__error_blast_radius", easyjson.NewJSON(blastRadius))
	propagateErrorPayload.SetByPath("error.error", easyjson.NewJSON(false))
	propagateErrorPayload.SetByPath("error.error_distribution", easyjson.NewJSON(false))

	if amI, ok := amIPatientZero(ctx, dbc, currentObject); amI {
		if !ok {
			return nil
		}
		propagateErrorPayload.SetByPath("error.error", easyjson.NewJSON(true))
	}
	if isAnyOfMyNeighbourInfected(ctx, dbc, relyLinkedIDs, blastRadius) {
		propagateErrorPayload.SetByPath("error.error_distribution", easyjson.NewJSON(true))
	}

	if err := dbc.CMDB.ObjectUpdate(ctx.Self.ID, propagateErrorPayload, false); err != nil {
		lg.Logf(lg.ErrorLevel, "PropagateError: cant update object %s: %v", ctx.Self.ID, err)
	}

	return propagateLinkedIDs
}

func amIPatientZero(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, currentObject easyjson.JSON) (bool, bool) {
	types, err := ctx.GetObjectImplTypes()
	if err != nil || len(types) == 0 {
		lg.Logf(lg.DebugLevel, "PropagateError: failed to get types from %s: %v", ctx.Self.ID, err)
		return false, false
	}

	switch ctx.Domain.GetObjectIDWithoutDomain(types[0]) {
	case "foliage-cn_ad-k8s-pod":
		if currentObject.GetByPath("body.restartCount").AsNumericDefault(0) > 0 {
			return true, true
		}
	}

	return false, true
}

func isAnyOfMyNeighbourInfected(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, linkedIDs []string, blastRadius int) bool {
	for _, id := range linkedIDs {
		nighbourObject, err := dbc.CMDB.ObjectRead(id)
		if err != nil {
			lg.Logf(lg.ErrorLevel, "PropagateError: can't read neightbour object: %s, skipping", err)
			continue
		}

		existingBlastRadiusFloat := nighbourObject.GetByPath("body.error.__error_blast_radius").AsNumericDefault(0)
		if int(existingBlastRadiusFloat) >= blastRadius { // Skipping this neighbour, its blast radius is the same or more distant
			continue
		}

		hasError := nighbourObject.GetByPath("body.error.error").AsBoolDefault(false)
		hasErrorDistribution := nighbourObject.GetByPath("body.error.error_distribution").AsBoolDefault(false)

		if hasError || hasErrorDistribution {
			return true
		}
	}
	return false
}

func getLinked(ctx *sfPlugins.StatefunContextProcessor, dbc db.DBSyncClient, linkTags []string) []string {
	allLinkedIDs := []string{}
	for _, tag := range linkTags {
		query := fmt.Sprintf(".*[l:tags('%s')]", tag)
		linkedIDs, err := dbc.Query.JPGQLCtraQuery(ctx.Self.ID, query)
		lg.Logf(lg.DebugLevel, "PropagateError: outgoing linkedIDs for clearing from %s: %v", ctx.Self.ID, linkedIDs)
		if err == nil && len(linkedIDs) > 0 {
			allLinkedIDs = append(allLinkedIDs, linkedIDs...)
		}
	}
	return allLinkedIDs
}

func propagateToLinked(ctx *sfPlugins.StatefunContextProcessor, linkedIDs []string, errorTS int64, blastRadius int) {
	for _, linkedID := range linkedIDs {
		if linkedID == ctx.Self.ID || strings.HasSuffix(linkedID, ctx.Self.ID) {
			continue
		}

		newPayload := easyjson.NewJSONObject()
		newPayload.SetByPath("__error_timestamp_nano", easyjson.NewJSON(errorTS))
		newPayload.SetByPath("__error_blast_radius", easyjson.NewJSON(blastRadius+1))

		system.MsgOnErrorReturn(ctx.Signal(
			sfPlugins.AutoSignalSelect,
			PropagateErrorFunctionName,
			ctx.Domain.GetObjectIDByShadowObjectID(linkedID),
			&newPayload,
			nil,
		))
	}
}
