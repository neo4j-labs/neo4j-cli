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

func TestInstallCmd_Table(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code", "cursor")

	require.NoError(t, f.exec(t, "install"))

	out := f.stdout.String()
	// Both agents listed with the "installed" action.
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "cursor")
	assert.Contains(t, out, "installed")
	// Verify on-disk side-effect (sanity check the underlying installer ran).
	cc := skill.FindAgent("claude-code")
	sp, _ := cc.SkillsPath()
	exists, _ := afero.Exists(f.fs, filepath.Join(sp, testSkillName, "SKILL.md"))
	assert.True(t, exists)
}

func TestInstallCmd_JSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")

	require.NoError(t, f.exec(t, "install"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "claude-code", rows[0]["agent"])
	assert.Equal(t, "Claude Code", rows[0]["display_name"])
	assert.Equal(t, "installed", rows[0]["action"])
	assert.Contains(t, rows[0]["skills_path"].(string), testSkillName)
}

func TestInstallCmd_AgentFlag(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code", "cursor")

	require.NoError(t, f.exec(t, "install", "--agent", "Claude-Code")) // case-insensitive

	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.NotContains(t, out, "cursor", "single-agent install must not touch cursor")
}

func TestInstallCmd_PositionalSelf(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")

	require.NoError(t, f.exec(t, "install", "self"))
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "installed")
}

func TestInstallCmd_PositionalBinaryNameAlias(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")

	require.NoError(t, f.exec(t, "install", testSkillName))
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "installed")
}

func TestInstallCmd_PositionalAgentHardBreak(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")

	err := f.exec(t, "install", "claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: claude-code")
	assert.Contains(t, err.Error(), "did you mean '--agent claude-code'?")
}

func TestInstallCmd_PositionalUnknownSkill(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")

	err := f.exec(t, "install", "no-such-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: no-such-skill")
	assert.NotContains(t, err.Error(), "did you mean", "non-agent unknown skill must not suggest --agent")
}

func TestInstallCmd_AgentFlagUnknown(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")

	err := f.exec(t, "install", "--agent", "vscode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
	assert.Contains(t, err.Error(), "valid agents:")
}

func TestInstallCmd_NoAgentsDetected(t *testing.T) {
	f := newFixture(t, "/home/alice", "default") // no agents detected

	err := f.exec(t, "install")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no supported agents detected")
}

func TestInstallCmd_HelpListsAgents(t *testing.T) {
	f := newFixture(t, "/home/alice", "default")

	require.NoError(t, f.exec(t, "install", "--help"))

	out := f.stdout.String()
	assert.Contains(t, out, "Supported agents:")
	for _, a := range skill.AGENTS {
		assert.Contains(t, out, a.Name, "help output must list agent %q", a.Name)
	}
	// Example block surfaces both forms.
	assert.Contains(t, out, "Examples:")
	assert.Contains(t, out, "skill install")
	assert.Contains(t, out, "--agent claude-code")
	// The --agent flag must appear in --help.
	assert.Contains(t, out, "--agent")
}
