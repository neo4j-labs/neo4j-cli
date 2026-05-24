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

func TestListCmd_ConductorPartialInstallJSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "conductor")
	codex := skill.FindAgent("codex")
	sp, _ := codex.SkillsPath()
	skillFile := filepath.Join(sp, testSkillName, "SKILL.md")
	require.NoError(t, f.fs.MkdirAll(filepath.Dir(skillFile), 0755))
	require.NoError(t, afero.WriteFile(f.fs, skillFile, []byte("---\nversion: 1.7.0\n---\n"), 0600))

	require.NoError(t, f.exec(t, "list"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	var conductor map[string]any
	for _, row := range rows {
		if row["agent"] == "conductor" {
			conductor = row
			break
		}
	}
	require.NotNil(t, conductor)
	assert.Equal(t, true, conductor["detected"])
	assert.Equal(t, true, conductor["installed"])
	assert.Equal(t, "codex:1.7.0,claude-code:missing", conductor["installed_version"])
	require.Len(t, conductor["install_details"], 2)
}

func TestListCmd_JSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")
	require.NoError(t, f.exec(t, "install", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "list"))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	// Catalog length — should include every supported agent.
	assert.Len(t, rows, len(skill.AGENTS))

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
