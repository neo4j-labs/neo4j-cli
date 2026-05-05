// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCmd_Table(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "install", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "list"))
	out := f.stdout.String()
	lower := strings.ToLower(out)
	assert.Contains(t, out, "claude-code")
	// Header columns present (case-insensitive — go-pretty upper-cases).
	assert.Contains(t, lower, "detected")
	assert.Contains(t, lower, "installed")
	// claude-code shows installed=yes; another agent (e.g. cursor) shows no.
	assert.Contains(t, out, "yes")
	assert.Contains(t, out, "no")
	// Installed version threaded through.
	assert.Contains(t, out, "1.7.0")
}

func TestListCmd_JSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")
	require.NoError(t, f.exec(t, "install", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "list"))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	// Catalog length — should include all 10 agents.
	assert.Len(t, rows, 10)

	// claude-code entry should have detected/installed = true and version.
	var cc map[string]any
	for _, r := range rows {
		if r["agent"] == "claude-code" {
			cc = r
			break
		}
	}
	require.NotNil(t, cc)
	assert.Equal(t, true, cc["detected"])
	assert.Equal(t, true, cc["installed"])
	assert.Equal(t, "1.7.0", cc["installed_version"])
}
