// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_RegistersLeaf(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	mcpGroup := findSubcommand(root, "mcp")
	require.NotNil(t, mcpGroup)

	check := findSubcommand(mcpGroup, "check")
	require.NotNil(t, check, "mcp check must be registered")
	assert.False(t, check.Hidden)
}

func TestCheck_NoInstalledServers(t *testing.T) {
	// With no agents installed and a pipe stdout, the output is JSON.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "check")
	require.NoError(t, err, "stderr=%s", stderr.String())
	// JSON array or friendly message depending on TTY; either is fine.
	t.Logf("check output: %s", stdout.String())
}

func TestCheck_JSONOutput(t *testing.T) {
	// JSON output should be a valid array.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "check", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))
	_ = rows
}

func TestCheck_JSONFieldsAreSnakeCase(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, true, "mcp", "check", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))

	for _, row := range rows {
		for key := range row {
			assert.Regexp(t, `^[a-z][a-z0-9_]*$`, key, "JSON key %q must be snake_case", key)
		}
	}
}

func TestCheck_ExitInfo(t *testing.T) {
	// mcp check is not write-annotated, so it should work without --rw.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "check")
	require.NoError(t, err, "check should not need --rw")
	_ = stdout
	_ = stderr
}
