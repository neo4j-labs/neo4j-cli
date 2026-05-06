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

func TestPrintCmd_RejectsPositionalArg(t *testing.T) {
	f := newFixture(t, "/home/alice", "table")

	err := f.exec(t, "print", "extra")
	require.Error(t, err)
}
