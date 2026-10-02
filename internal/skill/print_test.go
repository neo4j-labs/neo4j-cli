// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintCmd_RawSkillMd(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	require.NoError(t, f.exec(t, "print"))

	out := f.stdout.String()
	// Placeholder must remain literal — no {{VERSION}} substitution at print time.
	assert.Contains(t, out, "version: {{VERSION}}")
	assert.NotContains(t, out, "version: 1.7.0")
}

func TestPrintCmd_PositionalSelf(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	require.NoError(t, f.exec(t, "print", "self"))
	out := f.stdout.String()
	assert.Contains(t, out, "name: neo4j-cli")
}

func TestPrintCmd_PositionalBinaryNameAlias(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	require.NoError(t, f.exec(t, "print", testSkillName))
	out := f.stdout.String()
	assert.Contains(t, out, "name: neo4j-cli")
}

func TestPrintCmd_PositionalAgentHardBreak(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "print", "claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: claude-code")
	assert.Contains(t, err.Error(), "did you mean '--agent claude-code'?")
}

func TestPrintCmd_PositionalUnknownSkill(t *testing.T) {
	// Warm cache without the requested skill — the lookup falls through
	// to the unknown-skill error (no agent collision, cache populated).
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	err := f.exec(t, "print", "no-such-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: no-such-skill")
	assert.NotContains(t, err.Error(), "did you mean")
}

func TestPrintCmd_PositionalCatalogSkill_FromCache(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")
	seedCatalogCache(t, f.fs, "1.0.0", "neo4j-cypher-skill")

	require.NoError(t, f.exec(t, "print", "neo4j-cypher-skill"))
	out := f.stdout.String()
	assert.Contains(t, out, "name: neo4j-cypher-skill")
	assert.Contains(t, out, "cached neo4j-cypher-skill")
	// Print never hits the network.
	assert.Equal(t, 0, cs.pluginHits)
	assert.Equal(t, 0, cs.tarballHits)
}

func TestPrintCmd_PositionalCatalogSkill_ColdCache_RefreshHint(t *testing.T) {
	cs := newCatalogServer(t)
	withCatalogSeams(t, cs.doer())

	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "print", "neo4j-cypher-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: neo4j-cypher-skill")
	assert.Contains(t, err.Error(), "neo4j-cli skill refresh")
	// Print never hits the network even on cold cache.
	assert.Equal(t, 0, cs.pluginHits)
	assert.Equal(t, 0, cs.tarballHits)
}

func TestPrintCmd_RejectsExtraArgs(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "print", "self", "extra")
	require.Error(t, err)
}
