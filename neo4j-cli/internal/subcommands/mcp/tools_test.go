// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requiredToolFields are the snake_case keys every rendered tool row carries,
// per the OUTPUT casing rule (CLI-127).
var requiredToolFields = []string{
	"name", "title", "description",
	"read_only_hint", "idempotent_hint", "destructive_hint", "open_world_hint",
}

func TestTools_JSONManifest(t *testing.T) {
	stdout, stderr, err := runApp(t, true, "mcp", "tools", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	// No tool definitions exist yet, so the honest current contract is an empty
	// JSON array — never `null`, which would not decode as a list. The
	// per-row assertions below are the contract each definition must meet as
	// they are added.
	assert.Equal(t, "[]", strings.TrimSpace(stdout.String()))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows),
		"the tool list must be a JSON array; got %s", stdout.String())

	for _, row := range rows {
		for _, field := range requiredToolFields {
			assert.Contains(t, row, field)
		}
		assert.Regexp(t, `^neo4j_cli_[a-z0-9_]+$`, row["name"])
	}

	// The SDK's Tool struct carries camelCase wire tags; leaking them here would
	// mean the snake_case projection was bypassed.
	assert.NotContains(t, stdout.String(), "readOnlyHint")
	assert.NotContains(t, stdout.String(), "inputSchema")
}

// TestTools_TableRendersSnakeCaseHeaders pins the rendered column set, which is
// the part of the table view that exists independently of any tool definition.
func TestTools_TableRendersSnakeCaseHeaders(t *testing.T) {
	stdout, stderr, err := runApp(t, true, "mcp", "tools", "--format", "table")
	require.NoError(t, err, "stderr=%s", stderr.String())

	// go-pretty upper-cases header cells, so match the snake_case shape only.
	for _, header := range []string{"NAME", "TITLE", "READ_ONLY_HINT", "DESTRUCTIVE_HINT", "DESCRIPTION"} {
		assert.Contains(t, stdout.String(), header)
	}
	assert.NotContains(t, stdout.String(), "READONLYHINT")
}

func TestTools_ToonRenders(t *testing.T) {
	stdout, stderr, err := runApp(t, true, "mcp", "tools", "--format", "toon")
	require.NoError(t, err, "stderr=%s", stderr.String())
	assert.NotContains(t, stdout.String(), "readOnlyHint")
}

func TestTools_RejectsPositionalArgs(t *testing.T) {
	_, _, err := runApp(t, true, "mcp", "tools", "extra")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}
