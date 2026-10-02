// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolBudgetCeiling enforces REQ-NF-001: the serialized tool-definition surface
// must fit a single prompt context window without crowding out the conversation.
// The CLI-218 concern was "if you start adding lots of tools it adds too much
// context". The ceiling was set against the measured 3259 B (~905 token) surface
// at task-014. Adding a tool requires either shrinking existing descriptions or
// an explicit budget decision.
const toolBudgetCeiling = 4000

func TestToolDefinitions_Budget(t *testing.T) {
	data, err := json.Marshal(ToolRows(ToolDefinitions()))
	require.NoError(t, err)

	size := len(data)
	t.Logf("tool-definition surface (mcp tool --format json): %d bytes", size)

	assert.LessOrEqual(t, size, toolBudgetCeiling,
		"Tool definitions (%d bytes) exceed the %d-byte budget (~1.1k tokens). "+
			"Adding a tool requires either shrinking existing descriptions or an "+
			"explicit budget decision. Measured size recorded in test log above for drift visibility.",
		size, toolBudgetCeiling)
}

func TestToolDefinitions_Naming(t *testing.T) {
	toolNameRe := regexp.MustCompile(`^neo4j_cli_[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	schemaPropRe := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	for _, tool := range ToolDefinitions() {
		t.Run(tool.Name, func(t *testing.T) {
			assert.Regexp(t, toolNameRe, tool.Name,
				"Tool name must match ^neo4j_cli_[a-z][a-z0-9]*(_[a-z0-9]+)*$")

			schema, _ := tool.InputSchema.(map[string]any)
			props, _ := schema["properties"].(map[string]any)
			for key := range props {
				assert.Regexp(t, schemaPropRe, key,
					"Schema property %q in tool %q must match ^[a-z][a-z0-9_]*$",
					key, tool.Name)
			}
		})
	}
}
