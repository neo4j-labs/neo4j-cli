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
	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "print", "no-such-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown skill: no-such-skill")
}

func TestPrintCmd_RejectsExtraArgs(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "print", "self", "extra")
	require.Error(t, err)
}
