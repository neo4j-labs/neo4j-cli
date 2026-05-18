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
func RenameResponseField(data api.ResponseData, from, to string) api.ResponseData {
	switch d := data.(type) {
	case api.SingleValueResponseData:
		return api.NewSingleValueResponseData(renameMapKey(d.Data, from, to))
	default:
		rows := data.AsArray()
		renamed := make([]map[string]any, len(rows))
		for i, row := range rows {
			renamed[i] = renameMapKey(row, from, to)
		}
		return api.NewListResponseData(renamed)
	}
}

// renameMapKey copies m and returns a new map where the key `from` is replaced
// by `to`. If `from` is absent the map is returned unchanged (still a copy).
func renameMapKey(m map[string]any, from, to string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == from {
			out[to] = v
		} else {
			out[k] = v
		}
	}
	return out
}
