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

func TestCheckCmd_AllOk_ExitsZero(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	require.NoError(t, f.exec(t, "install", "--all"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 2, "self + one catalog skill on claude-code")
	for _, r := range rows {
		assert.Equal(t, "ok", r["status"])
	}
}

func TestCheckCmd_Drift_ExitsNonZero(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))

	// Mutate the installed self-skill SKILL.md to a stale version.
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

func TestCheckCmd_UnknownVersion_ExitsNonZero(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")

	cc := skill.FindAgent("claude-code")
	sp, _ := cc.SkillsPath()
	skillFile := filepath.Join(sp, testSkillName, "SKILL.md")
	require.NoError(t, f.fs.MkdirAll(filepath.Dir(skillFile), 0755))
	// Frontmatter without a version line.
	require.NoError(t, afero.WriteFile(f.fs, skillFile, []byte("---\nname: x\n---\nbody\n"), 0600))

	err := f.exec(t, "check")
	require.Error(t, err, "unknown-version must exit non-zero")
	assert.Contains(t, err.Error(), "drift")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "unknown-version", rows[0]["status"])
	assert.Equal(t, "", rows[0]["installed_version"])
}

func TestCheckCmd_CatalogSkillDrift(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill"))

	// Plant drift on the catalog install.
	cc := skill.FindAgent("claude-code")
	sp, _ := cc.SkillsPath()
	skillFile := filepath.Join(sp, "neo4j-cypher-skill", "SKILL.md")
	require.NoError(t, afero.WriteFile(f.fs, skillFile, []byte("---\nname: neo4j-cypher-skill\nversion: 0.0.1\n---\n"), 0600))
	f.resetBuffers()

	err := f.exec(t, "check")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drift")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))

	var cypherRow map[string]any
	for _, r := range rows {
		if r["skill"] == "neo4j-cypher-skill" {
			cypherRow = r
			break
		}
	}
	require.NotNil(t, cypherRow, "catalog row missing")
	assert.Equal(t, "drift", cypherRow["status"])
	assert.Equal(t, "0.0.1", cypherRow["installed_version"])
	assert.Equal(t, "1.0.0", cypherRow["current_version"])
}

// TestCheckCmd_CatalogVersionFromSkillFile proves the available
// (current_version) value is sourced from the catalog skill's own cached
// SKILL.md `version:` line, NOT from the plugin.json top-level version. The
// fixture deliberately sets them apart (SKILL.md 2.5.0 vs plugin.json 1.0.0);
// if sourcing regressed to plugin.json this assertion would see 1.0.0.
func TestCheckCmd_CatalogVersionFromSkillFile(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCacheWithSkillVersion(t, f.fs, "1.0.0", "2.5.0", "neo4j-cypher-skill")
	require.NoError(t, f.exec(t, "install", "neo4j-cypher-skill"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))

	var cypherRow map[string]any
	for _, r := range rows {
		if r["skill"] == "neo4j-cypher-skill" {
			cypherRow = r
			break
		}
	}
	require.NotNil(t, cypherRow, "catalog row missing")
	assert.Equal(t, "2.5.0", cypherRow["current_version"], "current_version must come from the skill's SKILL.md, not plugin.json")
	assert.Equal(t, "2.5.0", cypherRow["installed_version"], "install preserves the upstream SKILL.md version verbatim")
	assert.Equal(t, "ok", cypherRow["status"])
}

func TestCheckCmd_NoneInstalled(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "check"))
	out := f.stdout.String()
	assert.Contains(t, strings.ToLower(out), "no installed skills")
}

func TestCheckCmd_RefreshForcesFetch(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.0.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "check", "--refresh"))
	assert.Equal(t, 1, cs.pluginHits, "--refresh must force a plugin.json fetch")
}

func TestCheckCmd_TableContainsAllColumns(t *testing.T) {
	f := newFixture(t, "/home/alice", "table", "claude-code")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))
	lower := strings.ToLower(f.stdout.String())
	for _, col := range []string{"skill", "agent", "installed_version", "current_version", "status"} {
		assert.Contains(t, lower, col, "table header must include %q", col)
	}
}

func TestCheckCmd_ToonContainsAllColumns(t *testing.T) {
	f := newFixture(t, "/home/alice", "toon", "claude-code")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))
	out := f.stdout.String()
	for _, col := range []string{"skill", "agent", "installed_version", "current_version", "status"} {
		assert.Contains(t, out, col, "toon must include %q key", col)
	}
}

func TestCheckCmd_JSON(t *testing.T) {
	f := newFixture(t, "/home/alice", "json", "claude-code")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "check"))
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, testSkillName, rows[0]["skill"])
	assert.Equal(t, "claude-code", rows[0]["agent"])
	assert.Equal(t, "ok", rows[0]["status"])
	assert.Equal(t, "1.7.0", rows[0]["installed_version"])
	assert.Equal(t, "1.7.0", rows[0]["current_version"])
}
