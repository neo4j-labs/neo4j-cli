// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
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
	assert.NotNil(t, install.Flags().Lookup("allow-writes"), "--allow-writes flag must exist")
	assert.NotNil(t, install.Flags().Lookup(server.AllowAuraFlag), "--allow-aura flag must exist")
	assert.NotNil(t, install.Flags().Lookup(server.AllowCredentialWriteFlag), "--allow-credential-write flag must exist")
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

// TestResolveInstallGates_CoverageMatrix exercises resolveInstallGates against
// all 6 matrix rows from CLI-236 task-004, including the two that would fail
// under a Changed-based --rw implementation: --rw=false and no --rw at all.
//
// Row guide:
//
//	1  --rw=true  only         → all three = true  (--rw VALUE)
//	2  --rw=false only         → all three = false (--rw VALUE)
//	3  no flags (TTY default)  → all three = false (--rw VALUE)
//	4  --allow-writes=true     → literal true/false/false
//	5  --allow-aura=true       → literal false/true/false
//	6  --allow-cred-write=true → literal false/false/true
func TestResolveInstallGates_CoverageMatrix(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want skill.MCPGates
	}{
		{
			name: "--rw=true only: all three follow --rw value",
			args: []string{"--rw=true"},
			want: skill.MCPGates{AllowWrites: true, AllowAura: true, AllowCredentialWrite: true},
		},
		{
			name: "--rw=false only: all three follow --rw value",
			args: []string{"--rw=false"},
			want: skill.MCPGates{AllowWrites: false, AllowAura: false, AllowCredentialWrite: false},
		},
		{
			name: "no flags: all default to false",
			args: []string{},
			want: skill.MCPGates{AllowWrites: false, AllowAura: false, AllowCredentialWrite: false},
		},
		{
			name: "--allow-writes=true: literal values, --rw ignored for aura+cred",
			args: []string{"--rw=true", "--allow-writes=true"},
			want: skill.MCPGates{AllowWrites: true, AllowAura: false, AllowCredentialWrite: false},
		},
		{
			name: "--allow-aura=true: literal values, --rw ignored for writes+cred",
			args: []string{"--rw=true", "--allow-aura=true"},
			want: skill.MCPGates{AllowWrites: false, AllowAura: true, AllowCredentialWrite: false},
		},
		{
			name: "--allow-credential-write=true: literal values, --rw ignored for writes+aura",
			args: []string{"--rw=true", "--allow-credential-write=true"},
			want: skill.MCPGates{AllowWrites: false, AllowAura: false, AllowCredentialWrite: true},
		},
		{
			name: "two overrides: both honoured, third stays false",
			args: []string{"--rw=true", "--allow-aura=true", "--allow-writes=true"},
			want: skill.MCPGates{AllowWrites: true, AllowAura: true, AllowCredentialWrite: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			flags.RegisterRwFlag(cmd)
			cmd.Flags().Bool("allow-writes", false, "")
			cmd.Flags().Bool(server.AllowAuraFlag, false, "")
			cmd.Flags().Bool(server.AllowCredentialWriteFlag, false, "")
			require.NoError(t, cmd.ParseFlags(tt.args))

			got := mcp.ResolveInstallGates(cmd)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestInstall_WritesResolvedGatesToConfig runs the real command end to end and
// asserts the env block that lands in the agent's config file. The matrix test
// above covers resolveInstallGates in isolation; this one covers the wiring
// from flags through InstallMCPConfig to disk, which is what actually decides
// whether Claude Desktop can perform writes.
func TestInstall_WritesResolvedGatesToConfig(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		wantWrites          string
		wantAura            string
		wantCredentialWrite string
	}{
		{
			name:                "--rw enables all three gates",
			args:                []string{"mcp", "install", "--agent", "claude-desktop", "--rw"},
			wantWrites:          "true",
			wantAura:            "true",
			wantCredentialWrite: "true",
		},
		{
			name:                "--allow-writes scopes to writes only",
			args:                []string{"mcp", "install", "--agent", "claude-desktop", "--rw", "--allow-writes"},
			wantWrites:          "true",
			wantAura:            "false",
			wantCredentialWrite: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, root, stdout, stderr := newMCPInstallFixtureFS(t, true)
			root.SetArgs(tt.args)
			require.NoError(t, root.Execute(), "stderr=%s stdout=%s", stderr.String(), stdout.String())

			a := skill.FindAgent("claude-desktop")
			require.NotNil(t, a)
			configPath, ok := a.MCPConfigPath()
			require.True(t, ok)

			raw, err := afero.ReadFile(fs, configPath)
			require.NoError(t, err)

			var cfg map[string]any
			require.NoError(t, json.Unmarshal(raw, &cfg))

			servers, ok := cfg["mcpServers"].(map[string]any)
			require.True(t, ok, "mcpServers must exist")
			entry, ok := servers["neo4j-cli"].(map[string]any)
			require.True(t, ok, "neo4j-cli entry must exist")
			env, ok := entry["env"].(map[string]any)
			require.True(t, ok, "env block must exist")

			assert.Len(t, env, 5)
			assert.Equal(t, "1", env[skill.EnvMCPFeatureFlag])
			assert.Equal(t, "1", env[skill.EnvMCPManifest])
			assert.Equal(t, tt.wantWrites, env[skill.EnvMCPAllowWrites])
			assert.Equal(t, tt.wantAura, env[skill.EnvMCPAllowAura])
			assert.Equal(t, tt.wantCredentialWrite, env[skill.EnvMCPAllowCredentialWrite])
		})
	}
}
