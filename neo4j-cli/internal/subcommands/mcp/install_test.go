// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_RegistersLeaf(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	mcpGroup := findSubcommand(root, "mcp")
	require.NotNil(t, mcpGroup)

	install := findSubcommand(mcpGroup, "install")
	require.NotNil(t, install, "mcp install must be registered")
	assert.False(t, install.Hidden)
	assert.True(t, install.Annotations["write"] == "true", "install must be write-annotated")
}

func TestInstall_HasWriteFlag(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	install := findSubcommand(findSubcommand(root, "mcp"), "install")
	require.NotNil(t, install)
	assert.Equal(t, "true", install.Annotations["write"])
}

func TestInstall_HasAgentAndBundleFlags(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	install := findSubcommand(findSubcommand(root, "mcp"), "install")
	require.NotNil(t, install)

	assert.NotNil(t, install.Flags().Lookup("agent"), "--agent flag must exist")
	assert.NotNil(t, install.Flags().Lookup("all"), "--all flag must exist")
	assert.NotNil(t, install.Flags().Lookup("bundle"), "--bundle flag must exist")
}

func TestInstall_RequiresRW(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "install", "--agent", "claude-desktop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--rw")
	_ = stdout
	_ = stderr
}

func TestInstall_NeedsAgentDetected(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "install", "--rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no MCP-capable agents detected")
	_ = stdout
	_ = stderr
}

func TestInstall_UnknownAgent(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "install", "--agent", "nope", "--rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown MCP agent")
	_ = stdout
	_ = stderr
}

func TestInstall_SkillOnlyAgentRefused(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, false, "mcp", "install", "--agent", "claude-code", "--rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill-only agent")
	_ = stdout
	_ = stderr
}

func TestInstall_SuccessOutput(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, true, "mcp", "install", "--agent", "claude-desktop", "--rw")
	require.NoError(t, err, "stderr=%s", stderr.String())

	t.Logf("stdout: %s", stdout.String())
	assert.Contains(t, stdout.String(), "claude-desktop")
	assert.Contains(t, stdout.String(), "config")
}
