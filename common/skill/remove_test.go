// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/skill"
)

func TestRemoveCmd_Table(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")

	require.NoError(t, f.exec(t, "install"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove"))
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "removed")

	// Verify on-disk: install dir gone.
	cc := skill.FindAgent("claude-code")
	sp, _ := cc.SkillsPath()
	exists, _ := afero.DirExists(f.fs, filepath.Join(sp, testSkillName))
	assert.False(t, exists)
}

func TestRemoveCmd_JSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")
	require.NoError(t, f.exec(t, "install"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove"))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "removed", rows[0]["action"])
}

func TestRemoveCmd_Idempotent(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	// Never installed.
	require.NoError(t, f.exec(t, "remove", "claude-code"))
	f.resetBuffers()
	// Second run still succeeds.
	require.NoError(t, f.exec(t, "remove", "claude-code"))
}

func TestRemoveCmd_UnknownAgent(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	err := f.exec(t, "remove", "vscode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}
