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
)

func TestRefreshCmd_HappyPath_FetchesAndRenders(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.0.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")

	require.NoError(t, f.exec(t, "refresh"))
	out := f.stdout.String()
	assert.Contains(t, out, "1.0.0")
	assert.Contains(t, out, "catalog")

	assert.Equal(t, 1, cs.pluginHits)
	assert.Equal(t, 1, cs.tarballHits, "cold cache must trigger tarball extract")

	plugin, err := afero.ReadFile(f.fs, filepath.Join(installCatalogCacheRoot, "plugin.json"))
	require.NoError(t, err)
	assert.Equal(t, cs.pluginBody, plugin)
	exists, _ := afero.Exists(f.fs, filepath.Join(installCatalogCacheRoot, "fetched-at"))
	assert.True(t, exists, "fetched-at must be written on successful refresh")
}

func TestRefreshCmd_NoOp_WhenVersionUnchanged(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.0.0", "neo4j-cypher-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "refresh"))

	assert.Equal(t, 1, cs.pluginHits, "refresh always fetches plugin.json")
	assert.Equal(t, 0, cs.tarballHits, "tarball must be skipped when version unchanged")
}

func TestRefreshCmd_NetworkFailure_WithCache_WarnsAndExitsZero(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "refresh"))

	assert.Contains(t, f.stderr.String(), "warning")
	assert.Contains(t, f.stderr.String(), "using cached content")
	out := f.stdout.String()
	assert.Contains(t, out, "1.0.0", "cached version must still be rendered")
}

func TestRefreshCmd_NetworkFailure_NoCache_Errors(t *testing.T) {
	cs := newCatalogServer(t)
	cs.failPlugin = true
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "refresh")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog refresh failed")
	assert.Contains(t, err.Error(), "no local cache")
}

func TestRefreshCmd_JSON(t *testing.T) {
	cs := newCatalogServer(t)
	cs.pluginBody = pluginJSONBody("1.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")
	cs.tarballBody = makeCatalogTarball(t, "1.0.0", "neo4j-cypher-skill", "neo4j-gds-skill")
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "json")

	require.NoError(t, f.exec(t, "refresh"))

	var row map[string]any
	require.NoError(t, json.Unmarshal(f.stdout.Bytes(), &row))
	assert.Equal(t, "1.0.0", row["version"])
	assert.Equal(t, float64(2), row["skill_count"])
	assert.Equal(t, "catalog", row["source"])
}

func TestRefreshCmd_RejectsExtraArgs(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "refresh", "extra")
	require.Error(t, err)
}

func TestRefreshCmd_AppearsInParentHelp(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	require.NoError(t, f.exec(t, "--help"))
	out := f.stdout.String()
	assert.Contains(t, out, "refresh", "skill --help must list the refresh leaf")
}
