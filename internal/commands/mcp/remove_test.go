// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemove_RegistersLeaf(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	mcpGroup := findSubcommand(root, "mcp")
	require.NotNil(t, mcpGroup)

	remove := findSubcommand(mcpGroup, "remove")
	require.NotNil(t, remove, "mcp remove must be registered")
	assert.False(t, remove.Hidden)
	assert.True(t, remove.Annotations["write"] == "true")
}

func TestRemove_HasAgentFlag(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	remove := findSubcommand(findSubcommand(root, "mcp"), "remove")
	require.NotNil(t, remove)
	assert.NotNil(t, remove.Flags().Lookup("agent"), "--agent flag must exist")
}

func TestRemove_RequiresRW(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "remove", "--agent", "claude-desktop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--rw")
	_ = stdout
	_ = stderr
}

func TestRemove_UnknownAgent(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "remove", "--agent", "nope", "--rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown MCP agent")
	_ = stdout
	_ = stderr
}

func TestRemove_SkillOnlyAgentRefused(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "remove", "--agent", "claude-code", "--rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill-only agent")
	_ = stdout
	_ = stderr
}

func TestRemove_IdempotentWithNoAgents(t *testing.T) {
	// Running remove when no agents detected should not error (idempotent).
	stdout, stderr, err := runMCPApp(t, false, "mcp", "remove", "--rw")
	require.NoError(t, err, "remove is idempotent even with no agents")

	// No output expected when no agents matched.
	_ = stdout
	_ = stderr
}

func TestRemove_SuccessOutput(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, true, "mcp", "remove", "--agent", "claude-desktop", "--rw")
	require.NoError(t, err, "stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "claude-desktop")
}
