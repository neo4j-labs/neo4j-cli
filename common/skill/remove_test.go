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
	f := newFixture(t, "/home/alice", "table", "claude-code")

	require.NoError(t, f.exec(t, "install"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove", "self"))
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "removed")
	// Reinstall hint emitted to stderr so JSON callers stay clean.
	assert.Contains(t, f.stderr.String(), "Run 'neo4j-cli skill install' to reinstall.")

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

	require.NoError(t, f.exec(t, "remove", "self"))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "removed", rows[0]["action"])
	// Hint must go to stderr so stdout stays valid JSON.
	assert.Contains(t, f.stderr.String(), "Run 'neo4j-cli skill install' to reinstall.")
}

func TestRemoveCmd_PositionalBinaryNameAlias(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "install"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove", testSkillName))
	out := f.stdout.String()
	assert.Contains(t, out, "removed")
	assert.Contains(t, f.stderr.String(), "Run 'neo4j-cli skill install' to reinstall.")
}

func TestRemoveCmd_PositionalSelfPrintsReinstallHint(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "remove", "self"))
	assert.Contains(t, f.stderr.String(), "Run 'neo4j-cli skill install' to reinstall.")
}

func TestRemoveCmd_PositionalCatalogSkill(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	// Install the catalog skill so there is something to remove.
	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove", "neo4j-cypher-skill"))
	out := f.stdout.String()
	assert.Contains(t, out, "neo4j-cypher-skill")
	assert.Contains(t, out, "removed")

	a := skill.FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	exists, _ := afero.DirExists(f.fs, filepath.Join(sp, "neo4j-cypher-skill"))
	assert.False(t, exists)
}

func TestRemoveCmd_AgentFlagScopes(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code", "cursor")
	require.NoError(t, f.exec(t, "install"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove", "self", "--agent", "Claude-Code")) // case-insensitive
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.NotContains(t, out, "cursor", "scoped remove must not touch cursor")
}

func TestRemoveCmd_Idempotent(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	// Never installed.
	require.NoError(t, f.exec(t, "remove", "self"))
	f.resetBuffers()
	// Second run still succeeds.
	require.NoError(t, f.exec(t, "remove", "self"))
}

func TestRemoveCmd_IdempotentCatalogSkillNotInstalled(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	// Never installed — removing the catalog skill is a no-op zero-exit.
	require.NoError(t, f.exec(t, "remove", "neo4j-cypher-skill"))
}

func TestRemoveCmd_MissingPositional(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	err := f.exec(t, "remove")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a [skill-name] positional or --all")
}

func TestRemoveCmd_PositionalAgentHardBreak(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	err := f.exec(t, "remove", "claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: claude-code")
	assert.Contains(t, err.Error(), "did you mean '--agent claude-code'?")
}

func TestRemoveCmd_PositionalUnknownSkill(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	err := f.exec(t, "remove", "no-such-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: no-such-skill")
}

func TestRemoveCmd_PositionalUnknownSkillWithCache(t *testing.T) {
	// Warm cache without the requested skill — the lookup should fail
	// with the unknown-skill error, not silently no-op.
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	err := f.exec(t, "remove", "no-such-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: no-such-skill")
}

func TestRemoveCmd_AgentFlagUnknown(t *testing.T) {
	f := newFixture(t, "/home/alice", "default", "claude-code")
	err := f.exec(t, "remove", "self", "--agent", "vscode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestRemoveCmd_AllRemovesCatalogPreservesSelf(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")

	// Install everything via --all so there is content to remove.
	require.NoError(t, f.exec(t, "install", "--all"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove", "--all"))

	a := skill.FindAgent("claude-code")
	sp, _ := a.SkillsPath()

	// Catalog skills are gone.
	for _, name := range []string{"neo4j-cypher-skill", "neo4j-gds-skill"} {
		exists, _ := afero.DirExists(f.fs, filepath.Join(sp, name))
		assert.Falsef(t, exists, "--all must remove catalog skill %s", name)
	}

	// Self-skill is preserved.
	selfExists, _ := afero.DirExists(f.fs, filepath.Join(sp, testSkillName))
	assert.True(t, selfExists, "--all must NOT remove the self-skill")

	// Omission note surfaced on stderr.
	assert.Contains(t, f.stderr.String(), "preserves the embedded self-skill")
}

func TestRemoveCmd_AllRejectsPositional(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	err := f.exec(t, "remove", "--all", "neo4j-cypher-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined")
}

func TestRemoveCmd_AllColdCacheIsNoop(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	// No seedCatalogCache — cache is cold.

	require.NoError(t, f.exec(t, "remove", "--all"))
	assert.Contains(t, f.stderr.String(), "skill catalog cache is empty")
}

func TestRemoveCmd_AllUnknownAgent(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	err := f.exec(t, "remove", "--all", "--agent", "vscode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown agent")
}

func TestRemoveCmd_AllAgentFlagScopes(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code", "cursor")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "install", "--all"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "remove", "--all", "--agent", "claude-code"))

	cc := skill.FindAgent("claude-code")
	ccSp, _ := cc.SkillsPath()
	exists, _ := afero.DirExists(f.fs, filepath.Join(ccSp, "neo4j-cypher-skill"))
	assert.False(t, exists, "scoped --all must remove from claude-code")

	cur := skill.FindAgent("cursor")
	curSp, _ := cur.SkillsPath()
	exists, _ = afero.DirExists(f.fs, filepath.Join(curSp, "neo4j-cypher-skill"))
	assert.True(t, exists, "scoped --all must leave cursor untouched")
}

func TestRemoveCmd_HelpListsAgents(t *testing.T) {
	f := newFixture(t, "/home/alice", "default")

	require.NoError(t, f.exec(t, "remove", "--help"))

	out := f.stdout.String()
	assert.Contains(t, out, "Supported agents:")
	for _, a := range skill.SkillAgents() {
		assert.Contains(t, out, a.Name, "help output must list agent %q", a.Name)
	}
	assert.NotContains(t, out, "claude-desktop", "MCP-only agents must not be advertised as skill targets")
	assert.Contains(t, out, "Examples:")
	assert.Contains(t, out, "skill remove self")
	assert.Contains(t, out, "--agent")
	assert.Contains(t, out, "--all")
}
