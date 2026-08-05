// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// This file is in the INTERNAL mcp package (unlike the rest of the tests) so it
// can exercise the unexported tool-definition projection directly.

package mcp

import (
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolRows_AsArray_HintDefaults(t *testing.T) {
	falsy := false

	rows := toolRows{
		{Name: "neo4j_cli_no_annotations"},
		{
			Name:        "neo4j_cli_read",
			Title:       "Read",
			Description: "reads",
			Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		},
		{
			Name: "neo4j_cli_write",
			Annotations: &mcpsdk.ToolAnnotations{
				DestructiveHint: &falsy,
				OpenWorldHint:   &falsy,
			},
		},
	}.AsArray()

	require.Len(t, rows, 3)

	// Absent annotations must widen to the MCP spec defaults, not to the Go
	// zero values: destructive and open-world default to true.
	assert.Equal(t, map[string]any{
		"name":             "neo4j_cli_no_annotations",
		"title":            "",
		"description":      "",
		"read_only_hint":   false,
		"idempotent_hint":  false,
		"destructive_hint": true,
		"open_world_hint":  true,
	}, rows[0])

	assert.Equal(t, true, rows[1]["read_only_hint"])
	assert.Equal(t, true, rows[1]["idempotent_hint"])
	assert.Equal(t, "Read", rows[1]["title"])
	assert.Equal(t, "reads", rows[1]["description"])

	assert.Equal(t, false, rows[2]["destructive_hint"])
	assert.Equal(t, false, rows[2]["open_world_hint"])
}

func TestToolRows_MarshalJSON_EmitsSnakeCaseArray(t *testing.T) {
	data, err := json.Marshal(toolRows{{Name: "neo4j_cli_run"}})
	require.NoError(t, err)
	assert.JSONEq(t, `[{
		"name": "neo4j_cli_run",
		"title": "",
		"description": "",
		"read_only_hint": false,
		"idempotent_hint": false,
		"destructive_hint": true,
		"open_world_hint": true
	}]`, string(data))
}

// An empty manifest must render as `[]`, never `null`: a null body would make
// `mcp tool --format json` undecodable as an array.
func TestToolRows_EmptyMarshalsAsArray(t *testing.T) {
	data, err := json.Marshal(toolRows(nil))
	require.NoError(t, err)
	assert.Equal(t, "[]", string(data))
}
