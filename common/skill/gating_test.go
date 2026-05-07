// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallCmd_RequiresRw(t *testing.T) {
	f := newRootFixture(t, "/home/alice", "table", "claude-code")

	err := f.execSkill(t, "install")
	require.Error(t, err)
	assert.Equal(t, "this command writes; pass --rw to allow it", err.Error())
}

func TestInstallCmd_WithRwInstalls(t *testing.T) {
	f := newRootFixture(t, "/home/alice", "table", "claude-code")

	require.NoError(t, f.execSkill(t, "install", "--rw"))
	assert.Contains(t, f.stdout.String(), "installed")
}
