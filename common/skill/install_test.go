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
	// Warm cache without the requested skill — the lookup should fail
	// with the unknown-skill error, not the agent did-you-mean.
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

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
	for _, a := range skill.SkillAgents() {
		assert.Contains(t, out, a.Name, "help output must list agent %q", a.Name)
	}
	assert.NotContains(t, out, "claude-desktop", "MCP-only agents must not be advertised as skill targets")
	// Example block surfaces both forms.
	assert.Contains(t, out, "Examples:")
	assert.Contains(t, out, "skill install")
	assert.Contains(t, out, "--agent claude-code")
	// The --agent flag must appear in --help.
	assert.Contains(t, out, "--agent")
	// New catalog-aware flags must surface.
	assert.Contains(t, out, "--all")
	assert.Contains(t, out, "--refresh")
}

func TestInstallCmd_PositionalCatalogSkill_FromCache(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill"))
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "neo4j-cypher-skill")

	a := skill.FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	skillFile := filepath.Join(sp, "neo4j-cypher-skill", "SKILL.md")
	data, err := afero.ReadFile(f.fs, skillFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "version: 1.0.0", "installer must inject the catalog version")

	// Warm cache + fresh fetched-at: no network hits.
	assert.Equal(t, 0, cs.pluginHits)
	assert.Equal(t, 0, cs.tarballHits)
}

func TestInstallCmd_AllInstallsSelfAndCatalog(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")

	require.NoError(t, f.exec(t, "install", "--all"))
	out := f.stdout.String()
	assert.Contains(t, out, "claude-code")

	a := skill.FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	for _, name := range []string{testSkillName, "neo4j-cypher-skill", "neo4j-gds-skill"} {
		skillFile := filepath.Join(sp, name, "SKILL.md")
		exists, _ := afero.Exists(f.fs, skillFile)
		assert.True(t, exists, "--all must install %s", name)
	}
}

func TestInstallCmd_AllRejectsPositional(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	err := f.exec(t, "install", "--all", "neo4j-cypher-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined")
}

func TestInstallCmd_RefreshForcesFetch(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.0.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	// Seed a stale cache (older version) so refresh actually re-extracts.
	seedCatalogCache(t, f.fs, "0.9.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill", "--refresh"))
	assert.Equal(t, 1, cs.pluginHits)
	assert.Equal(t, 1, cs.tarballHits, "version diff must trigger tarball re-extract")

	a := skill.FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	skillFile := filepath.Join(sp, "neo4j-cypher-skill", "SKILL.md")
	data, err := afero.ReadFile(f.fs, skillFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "version: 1.0.0")
}

func TestInstallCmd_ColdCache_NetworkFailure_Errors(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "default", "claude-code")

	err := f.exec(t, "install", "neo4j-cypher-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill catalog unavailable")
	assert.Contains(t, err.Error(), "neo4j-cli skill refresh")
}

func TestInstallCmd_WarmCache_NetworkFailure_Warns(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	// --refresh forces a fetch even though the cache is fresh — the
	// fetch fails but cached content remains usable.
	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill", "--refresh"))
	assert.Contains(t, f.stderr.String(), "warning")
	assert.Contains(t, f.stderr.String(), "using cached content")

	a := skill.FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	exists, _ := afero.Exists(f.fs, filepath.Join(sp, "neo4j-cypher-skill", "SKILL.md"))
	assert.True(t, exists, "warm-cache fallback must still install from cached content")
}

func TestInstallCmd_StaleCache_AutoRefreshes(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.0.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	// Override fetched-at to a time long in the past so Stale() returns true.
	require.NoError(t, afero.WriteFile(f.fs,
		filepath.Join(installCatalogCacheRoot, "fetched-at"),
		[]byte("2000-01-01T00:00:00Z"), 0600))

	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill"))
	assert.Equal(t, 1, cs.pluginHits, "stale cache must auto-refresh on install")
}
