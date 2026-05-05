// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/skill"
)

func TestCheckCmd_NoDrift(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "install", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))
	out := f.stdout.String()
	assert.Contains(t, out, "ok")
	assert.Contains(t, out, "1.7.0")
}

func TestCheckCmd_Drift(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "install", "claude-code"))

	// Mutate the installed SKILL.md to a stale version.
	cc := skill.FindAgent("claude-code")
	sp, _ := cc.SkillsPath()
	skillFile := filepath.Join(sp, testSkillName, "SKILL.md")
	require.NoError(t, afero.WriteFile(f.fs, skillFile, []byte("---\nversion: 0.1.0\n---\n"), 0600))
	f.resetBuffers()

	err := f.exec(t, "check")
	require.Error(t, err, "check must exit non-zero on drift")
	assert.Contains(t, err.Error(), "drift")

	out := f.stdout.String()
	assert.Contains(t, out, "drift")
	assert.Contains(t, out, "0.1.0")
}

func TestCheckCmd_JSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")
	require.NoError(t, f.exec(t, "install", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "ok", rows[0]["status"])
	assert.Equal(t, "1.7.0", rows[0]["installed_version"])
	assert.Equal(t, "1.7.0", rows[0]["current_version"])
}

func TestCheckCmd_NoneInstalled(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	// Never installed — Check returns no rows, drift=false, exits 0.
	require.NoError(t, f.exec(t, "check"))
	out := f.stdout.String()
	assert.Contains(t, strings.ToLower(out), "no installed skills")
}
