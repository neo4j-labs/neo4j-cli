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

// expectedRowCount returns the number of (skill × agent) rows for `n`
// catalog skills plus the always-present self row, across the full AGENTS
// catalog.
func expectedRowCount(catalogSkills int) int {
	return (1 + catalogSkills) * len(skill.AGENTS)
}

func TestListCmd_ColdCache_OnlySelfRows(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true // cold cache + no network = self-only fallback
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")

	require.NoError(t, f.exec(t, "list"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	assert.Len(t, rows, len(skill.AGENTS), "cold cache must show self rows only")
	for _, r := range rows {
		assert.Equal(t, testSkillName, r["skill"])
		assert.Equal(t, "embedded", r["source"])
		assert.Equal(t, "1.7.0", r["available_version"])
	}
	assert.Contains(t, f.stderr.String(), "skill refresh", "cold cache must hint at refresh")
}

func TestListCmd_WarmCache_SelfAndCatalogRows(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code", "cursor")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")

	require.NoError(t, f.exec(t, "list"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	assert.Len(t, rows, expectedRowCount(2))

	// Self rows come first.
	for i := 0; i < len(skill.AGENTS); i++ {
		assert.Equal(t, testSkillName, rows[i]["skill"])
		assert.Equal(t, "embedded", rows[i]["source"])
		assert.Equal(t, "1.7.0", rows[i]["available_version"])
	}
	// Followed by catalog rows in plugin.json order.
	cypherStart := len(skill.AGENTS)
	for i := cypherStart; i < cypherStart+len(skill.AGENTS); i++ {
		assert.Equal(t, "neo4j-cypher-skill", rows[i]["skill"])
		assert.Equal(t, "catalog", rows[i]["source"])
		assert.Equal(t, "1.0.0", rows[i]["available_version"])
	}
	gdsStart := cypherStart + len(skill.AGENTS)
	for i := gdsStart; i < gdsStart+len(skill.AGENTS); i++ {
		assert.Equal(t, "neo4j-gds-skill", rows[i]["skill"])
		assert.Equal(t, "catalog", rows[i]["source"])
	}

	// No network hits — warm cache served the catalog.
	assert.Equal(t, 0, cs.pluginHits)
	assert.Equal(t, 0, cs.tarballHits)
}

func TestListCmd_StatusReflectsInstallAndDrift(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code", "cursor")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))
	f.resetBuffers()

	// Plant a drift row: cursor has self-skill at an older version.
	cursor := skill.FindAgent("cursor")
	sp, _ := cursor.SkillsPath()
	driftBody := "---\nname: " + testSkillName + "\nversion: 0.0.1\n---\n# old\n"
	require.NoError(t, f.fs.MkdirAll(filepath.Join(sp, testSkillName), 0755))
	require.NoError(t, afero.WriteFile(f.fs, filepath.Join(sp, testSkillName, "SKILL.md"), []byte(driftBody), 0600))

	// And a row with no version frontmatter.
	cypherDir := filepath.Join(sp, "neo4j-cypher-skill")
	require.NoError(t, f.fs.MkdirAll(cypherDir, 0755))
	noVerBody := "---\nname: neo4j-cypher-skill\n---\n# no version\n"
	require.NoError(t, afero.WriteFile(f.fs, filepath.Join(cypherDir, "SKILL.md"), []byte(noVerBody), 0600))

	require.NoError(t, f.exec(t, "list"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))

	statuses := map[string]string{}
	for _, r := range rows {
		key := r["skill"].(string) + "@" + r["agent"].(string)
		statuses[key] = r["status"].(string)
	}
	assert.Equal(t, "installed", statuses[testSkillName+"@claude-code"], "freshly installed self → installed")
	assert.Equal(t, "drift", statuses[testSkillName+"@cursor"], "older self install → drift")
	assert.Equal(t, "unknown-version", statuses["neo4j-cypher-skill@cursor"], "no version frontmatter → unknown-version")
	assert.Equal(t, "not-installed", statuses["neo4j-cypher-skill@claude-code"], "absent install → not-installed")
}

func TestListCmd_Refresh_ForcesFetch(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.1.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.1.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "list", "--refresh"))
	assert.Equal(t, 1, cs.pluginHits)
	assert.Equal(t, 1, cs.tarballHits, "version diff must trigger tarball re-extract")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	cypherRow := findFirstRow(rows, "neo4j-cypher-skill", "claude-code")
	require.NotNil(t, cypherRow)
	assert.Equal(t, "1.1.0", cypherRow["available_version"], "refresh must surface the new catalog version")
}

func TestListCmd_StaleCache_AutoRefreshes(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.1.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.1.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	// Force staleness via the fetched-at marker.
	require.NoError(t, afero.WriteFile(f.fs,
		filepath.Join(installCatalogCacheRoot, "fetched-at"),
		[]byte("2000-01-01T00:00:00Z"), 0600))

	require.NoError(t, f.exec(t, "list"))
	assert.Equal(t, 1, cs.pluginHits, "stale cache must auto-refresh on list")
}

func TestListCmd_WarmCache_NetworkFailure_Warns(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	// --refresh forces a fetch which fails; warm cache should fall back.
	require.NoError(t, f.exec(t, "list", "--refresh"))
	assert.Contains(t, f.stderr.String(), "warning")
	assert.Contains(t, f.stderr.String(), "using cached content")

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	assert.Len(t, rows, expectedRowCount(1), "warm cache fallback must still list catalog rows")
}

func TestListCmd_TableContainsAllColumns(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "list"))
	lower := strings.ToLower(f.stdout.String())
	for _, col := range []string{"skill", "source", "agent", "detected", "installed", "installed_version", "available_version", "status"} {
		assert.Contains(t, lower, col, "table header must include %q", col)
	}
	assert.Contains(t, f.stdout.String(), testSkillName)
	assert.Contains(t, f.stdout.String(), "neo4j-cypher-skill")
	assert.Contains(t, f.stdout.String(), "embedded")
	assert.Contains(t, f.stdout.String(), "catalog")
}

func TestListCmd_ToonContainsAllColumns(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "toon", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "list"))
	out := f.stdout.String()
	for _, col := range []string{"skill", "source", "agent", "detected", "installed", "installed_version", "available_version", "status"} {
		assert.Contains(t, out, col, "toon must include %q key", col)
	}
}

// findFirstRow returns the first row matching (skill, agent) or nil.
func findFirstRow(rows []map[string]any, skillName, agentName string) map[string]any {
	for _, r := range rows {
		if r["skill"] == skillName && r["agent"] == agentName {
			return r
		}
	}
	return nil
}
