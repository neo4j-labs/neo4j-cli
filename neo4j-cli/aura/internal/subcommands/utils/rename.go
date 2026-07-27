// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import "github.com/neo4j/cli/neo4j-cli/aura/internal/api"

// RenameResponseField returns a new ResponseData with the key `from` renamed to
// `to` across every map in the response. The original data is not modified. If
// `from` is absent in a given map the map is left unchanged.
//
// The returned type mirrors the input: a single-item response remains a
// single-item response and a list response remains a list response, so that
// JSON output retains the same envelope shape ({"data":{...}} vs
// {"data":[...]}).
//
// This is used to surface `project_id` to the user in place of the API-level
// `tenant_id` field before handing the data to output.PrintBodyMap.
//
// RenameResponseField and NormalizeV2Beta1Response share the single
// copy-and-rename primitive renameMapKey. They differ only in tie-break: this
// helper unconditionally moves `from` onto `to` (used by non-migrated commands
// that only carry the legacy field), whereas NormalizeV2Beta1Response prefers an
// already-present native `to` (used by migrated v2beta1 commands whose responses
// may carry both).
func RenameResponseField(data api.ResponseData, from, to string) api.ResponseData {
	return mapResponse(data, func(m map[string]any) map[string]any {
		return renameMapKey(m, from, to, false)
	})
}

// mapResponse applies fn to every map in data, preserving the input's envelope
// shape (single-item stays single-item, list stays list) and leaving the
// original data unmodified.
func mapResponse(data api.ResponseData, fn func(map[string]any) map[string]any) api.ResponseData {
	switch d := data.(type) {
	case api.SingleValueResponseData:
		return api.NewSingleValueResponseData(fn(d.Data))
	default:
		rows := data.AsArray()
		mapped := make([]map[string]any, len(rows))
		for i, row := range rows {
			mapped[i] = fn(row)
		}
		return api.NewListResponseData(mapped)
	}
}

// renameMapKey is the single copy-and-rename primitive: it copies m and returns
// a new map where the key `from` is replaced by `to`. If `from` is absent the
// map is returned unchanged (still a copy). When preferTo is true and m already
// holds a native `to` value, that value is kept and `from` is simply dropped;
// when false, `from`'s value is moved onto `to` (overwriting any native `to`).
func renameMapKey(m map[string]any, from, to string, preferTo bool) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == from || k == to {
			continue
		}
		out[k] = v
	}
	if native, ok := m[to]; preferTo && ok {
		out[to] = native
	} else if legacy, ok := m[from]; ok {
		out[to] = legacy
	} else if native, ok := m[to]; ok {
		out[to] = native
	}
	return out
}
