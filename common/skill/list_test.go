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
// catalog skills plus the always-present self row, across the skill-capable
// agents. JSON output still emits one row per (skill × agent).
func expectedRowCount(catalogSkills int) int {
	return (1 + catalogSkills) * len(skill.SkillAgents())
}

func TestListCmd_ColdCache_OnlySelfRows(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true // cold cache + no network = self-only fallback
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json", "claude-code")

	require.NoError(t, f.exec(t, "list"))

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &rows))
	assert.Len(t, rows, len(skill.SkillAgents()), "cold cache must show self rows only")
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
	for i := 0; i < len(skill.SkillAgents()); i++ {
		assert.Equal(t, testSkillName, rows[i]["skill"])
		assert.Equal(t, "embedded", rows[i]["source"])
		assert.Equal(t, "1.7.0", rows[i]["available_version"])
	}
	// Followed by catalog rows in plugin.json order.
	cypherStart := len(skill.SkillAgents())
	for i := cypherStart; i < cypherStart+len(skill.SkillAgents()); i++ {
		assert.Equal(t, "neo4j-cypher-skill", rows[i]["skill"])
		assert.Equal(t, "catalog", rows[i]["source"])
		assert.Equal(t, "1.0.0", rows[i]["available_version"])
	}
	gdsStart := cypherStart + len(skill.SkillAgents())
	for i := gdsStart; i < gdsStart+len(skill.SkillAgents()); i++ {
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

func TestListCmd_Table_TwoSectionShape(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")
	require.NoError(t, f.exec(t, "install", "--agent", "claude-code"))
	f.resetBuffers()

	require.NoError(t, f.exec(t, "list"))
	out := f.stdout.String()

	assert.Contains(t, out, "Self-skill:", "table must include self-skill heading")
	assert.Contains(t, out, "Catalog:", "table must include catalog heading")

	// Self section: agent + per-agent install state columns; no skill/source.
	lower := strings.ToLower(out)
	for _, col := range []string{"agent", "detected", "installed", "installed_version", "available_version", "status"} {
		assert.Contains(t, lower, col, "table self section must include %q", col)
	}
	// Catalog section: skill + aggregated columns; installed_in is new.
	for _, col := range []string{"skill", "installed_in"} {
		assert.Contains(t, lower, col, "table catalog section must include %q", col)
	}

	assert.Contains(t, out, "neo4j-cypher-skill", "catalog skill must render")
}

func TestListCmd_Table_InstalledInFormatting(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	// Seed three catalog skills with different install patterns.
	f := newFixture(t, "/home/alice", "table", "claude-code", "cursor", "codex",
		"windsurf", "copilot", "antigravity", "gemini-cli", "cline", "pi", "opencode", "junie")
	seedCatalogCache(t, f.fs, "1.0.0", "skill-zero", "skill-some", "skill-all")

	// skill-some: installed on claude-code + codex.
	for _, name := range []string{"claude-code", "codex"} {
		a := skill.FindAgent(name)
		sp, _ := a.SkillsPath()
		dir := filepath.Join(sp, "skill-some")
		require.NoError(t, f.fs.MkdirAll(dir, 0755))
		body := "---\nname: skill-some\nversion: 1.0.0\n---\n# some\n"
		require.NoError(t, afero.WriteFile(f.fs, filepath.Join(dir, "SKILL.md"), []byte(body), 0600))
	}
	// skill-all: installed on every skill-capable agent.
	for _, a := range skill.SkillAgents() {
		sp, _ := a.SkillsPath()
		dir := filepath.Join(sp, "skill-all")
		require.NoError(t, f.fs.MkdirAll(dir, 0755))
		body := "---\nname: skill-all\nversion: 1.0.0\n---\n# all\n"
		require.NoError(t, afero.WriteFile(f.fs, filepath.Join(dir, "SKILL.md"), []byte(body), 0600))
	}
	// skill-zero: not installed anywhere.

	require.NoError(t, f.exec(t, "list"))
	out := f.stdout.String()

	// Pull the catalog section so we don't match the self-skill row.
	idx := strings.Index(out, "Catalog:")
	require.NotEqual(t, -1, idx, "catalog section must be present")
	catalog := out[idx:]

	assert.Contains(t, catalog, "skill-zero", "zero-installed catalog skill must render")
	assert.Contains(t, catalog, "not-installed", "zero-installed status must render")
	assert.Contains(t, catalog, "—", "zero-installed catalog skill must render '—'")

	assert.Contains(t, catalog, "skill-some", "partial catalog skill must render")
	assert.Contains(t, catalog, "partial", "partial status must render")
	assert.Contains(t, catalog, "2/11 (claude-code, codex)",
		"partial install must render 'N/11 (a, b)' in catalog order")

	assert.Contains(t, catalog, "skill-all", "fully installed catalog skill must render")
	assert.Contains(t, catalog, "11/11", "fully installed must render '11/11' without parenthetical")
	// Sanity: parenthetical must not appear on the all-installed row.
	skillAllLineIdx := strings.Index(catalog, "skill-all")
	require.NotEqual(t, -1, skillAllLineIdx)
	lineEnd := strings.Index(catalog[skillAllLineIdx:], "\n")
	require.NotEqual(t, -1, lineEnd)
	skillAllLine := catalog[skillAllLineIdx : skillAllLineIdx+lineEnd]
	assert.NotContains(t, skillAllLine, "(",
		"fully installed row must omit parenthetical")
}

func TestListCmd_Toon_TwoSectionShape(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "toon", "claude-code")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "list"))
	out := f.stdout.String()

	assert.Contains(t, out, "Self-skill:", "toon must include self-skill heading")
	assert.Contains(t, out, "Catalog:", "toon must include catalog heading")

	// Self section keys.
	for _, col := range []string{"agent", "detected", "installed", "installed_version", "available_version", "status"} {
		assert.Contains(t, out, col, "toon self section must include %q key", col)
	}
	// Catalog section keys.
	for _, col := range []string{"skill", "installed_in"} {
		assert.Contains(t, out, col, "toon catalog section must include %q key", col)
	}
}

func TestListCmd_Table_ColdCache_OnlySelfSection(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table", "claude-code")

	require.NoError(t, f.exec(t, "list"))
	out := f.stdout.String()

	assert.Contains(t, out, "Self-skill:", "cold cache must still render self section")
	assert.NotContains(t, out, "Catalog:", "cold cache must omit catalog section")
	assert.Contains(t, f.stderr.String(), "skill refresh", "cold cache hint must still fire")
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
