// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"encoding/json"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolDefinitions returns the tool definitions, in the order tools are
// advertised to clients. Both `mcp tools` and the server read them from here, so
// the printed surface cannot drift from the registered one.
func toolDefinitions() []*mcpsdk.Tool {
	return nil
}

// toolRows projects tool definitions into snake_case output rows. The SDK's own
// JSON tags are camelCase (`readOnlyHint`) because they are wire fields, which
// the repo's OUTPUT casing rule forbids for rendered output.
type toolRows []*mcpsdk.Tool

func (r toolRows) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, t := range r {
		// An absent hint is not "false": the MCP spec defaults destructiveHint
		// and openWorldHint to true, so a nil pointer must widen to the
		// pessimistic value rather than the zero value.
		row := map[string]any{
			"name":             t.Name,
			"title":            t.Title,
			"description":      t.Description,
			"read_only_hint":   false,
			"idempotent_hint":  false,
			"destructive_hint": true,
			"open_world_hint":  true,
		}
		if a := t.Annotations; a != nil {
			row["read_only_hint"] = a.ReadOnlyHint
			row["idempotent_hint"] = a.IdempotentHint
			if a.DestructiveHint != nil {
				row["destructive_hint"] = *a.DestructiveHint
			}
			if a.OpenWorldHint != nil {
				row["open_world_hint"] = *a.OpenWorldHint
			}
		}
		out = append(out, row)
	}
	return out
}

// MarshalJSON delegates to AsArray so the json and toon renderings emit the
// snake_case projection instead of the SDK struct's camelCase wire tags, and so
// an empty manifest renders as `[]` rather than `null`.
func (r toolRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}
