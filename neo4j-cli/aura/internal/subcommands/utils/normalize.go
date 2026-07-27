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
//
// This is RenameResponseField's superset for migrated commands: it applies the
// same tenant_id->project_id rename plus legacy_status->status, and — because a
// v2beta1 response may carry both the legacy and native field — routes through
// renameMapKey with the native-preferring tie-break (preferTo=true) rather than
// RenameResponseField's unconditional move.
func NormalizeV2Beta1Response(data api.ResponseData) api.ResponseData {
	return mapResponse(data, func(m map[string]any) map[string]any {
		out := renameMapKey(m, "legacy_status", "status", true)
		return renameMapKey(out, "tenant_id", "project_id", true)
	})
}
