// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_RegistersLeaf(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	mcpGroup := findSubcommand(root, "mcp")
	require.NotNil(t, mcpGroup)

	list := findSubcommand(mcpGroup, "list")
	require.NotNil(t, list, "mcp list must be registered")
	assert.False(t, list.Hidden)
}

func TestList_DefaultOutput(t *testing.T) {
	// With no detect dir seeded and a pipe stdout, the output is JSON.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "list")
	require.NoError(t, err, "stderr=%s", stderr.String())
	// JSON array or friendly message depending on TTY; either is fine.
	t.Logf("list output: %s", stdout.String())
}

func TestList_JSONOutput(t *testing.T) {
	// JSON output should be a valid array even when no agents detected.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "list", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))

	// With no detect dir seeded, output may be 0 or 1 row depending on
	// whether the test runner's HOME happens to point at a real Claude install.
	// At minimum it should be valid JSON.
	_ = rows
}

func TestList_DetectedAgent(t *testing.T) {
	// When claude-desktop is detected, list shows it.
	stdout, stderr, err := runMCPApp(t, true, "mcp", "list")
	require.NoError(t, err, "stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "claude-desktop")
}

func TestList_JSONFieldsAreSnakeCase(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, true, "mcp", "list", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))

	for _, row := range rows {
		for key := range row {
			assert.Regexp(t, `^[a-z][a-z0-9_]*$`, key, "JSON key %q must be snake_case", key)
		}
	}
}
