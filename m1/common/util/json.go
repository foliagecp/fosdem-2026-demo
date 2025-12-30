package util

import easyjson "github.com/foliagecp/easyjson"

// NormalizedEqual compares two JSON payloads in normalized form.
func NormalizedEqual(a, b easyjson.JSON) bool {
	return a.NormalizedClone().Equals(b.NormalizedClone())
}
