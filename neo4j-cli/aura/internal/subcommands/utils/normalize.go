// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import "github.com/neo4j/cli/neo4j-cli/aura/internal/api"

// NormalizeV2Beta1Response applies the v2beta1 output-contract mapping to an
// instance or session response so callers present a stable, v1-compatible
// shape to the user. It performs two renames across every map in the response:
//
//   - legacy_status -> status
//   - tenant_id     -> project_id
//
// This is the single shared normalization step for migrated v2beta1 commands so
// the field mapping is not duplicated per command. The original data is not
// modified; the returned type mirrors the input (single-item stays single-item,
// list stays list) so the JSON envelope shape is preserved.
//
// A native field always wins over the legacy source: if a map already contains
// `status`, its value is kept and any `legacy_status` is dropped; likewise a
// native `project_id` is kept over `tenant_id`.
func NormalizeV2Beta1Response(data api.ResponseData) api.ResponseData {
	switch d := data.(type) {
	case api.SingleValueResponseData:
		return api.NewSingleValueResponseData(normalizeMap(d.Data))
	default:
		rows := data.AsArray()
		normalized := make([]map[string]any, len(rows))
		for i, row := range rows {
			normalized[i] = normalizeMap(row)
		}
		return api.NewListResponseData(normalized)
	}
}

// normalizeMap returns a copy of m with legacy_status renamed to status and
// tenant_id renamed to project_id. A pre-existing native destination key
// (status / project_id) is preserved and the corresponding legacy key dropped.
func normalizeMap(m map[string]any) map[string]any {
	out := preferField(m, "legacy_status", "status")
	out = preferField(out, "tenant_id", "project_id")
	return out
}

// preferField copies m, ensuring `to` holds the native value if present,
// otherwise the value of `from`, and always drops `from` from the result.
func preferField(m map[string]any, from, to string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k == from {
			continue
		}
		out[k] = v
	}
	if native, ok := m[to]; ok {
		out[to] = native
	} else if legacy, ok := m[from]; ok {
		out[to] = legacy
	}
	return out
}
