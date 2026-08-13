// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServeCmd_FlagDefaults locks the read-only-by-default posture: every gate
// flag must default to the safe value so starting the server grants nothing.
func TestServeCmd_FlagDefaults(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	cmd := newServeCmd(cfg)

	for _, tc := range []struct{ flag, want string }{
		{"rw", "false"},
		{server.AllowAuraFlag, "false"},
		{server.AllowCredentialWriteFlag, "false"},
		{"format", "toon"},
	} {
		f := cmd.Flag(tc.flag)
		require.NotNil(t, f, "serve must register --%s", tc.flag)
		assert.Equal(t, tc.want, f.DefValue, "--%s default", tc.flag)
	}
}

// TestServeCmd_NotWriteAnnotated guards REQ-NF-006. Annotating serve as a write
// command would make EnforceWriteGate demand --rw merely to START the server,
// because stdout is never a TTY under MCP — destroying the read-only default.
func TestServeCmd_NotWriteAnnotated(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	cmd := newServeCmd(cfg)
	assert.NotEqual(t, "true", cmd.Annotations["write"],
		`serve must not be annotated write:"true"`)
}

// TestEnvBool covers the env fallback that lets a .mcpb manifest wire its
// settings-UI toggles through to the server, since Claude Desktop spawns the
// binary with env vars rather than flags the user can edit.
func TestEnvBool(t *testing.T) {
	const key = "NEO4J_CLI_MCP_TEST_ENVBOOL"

	assert.False(t, envBool(key), "unset must be false")

	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"1", true},
		{"TRUE", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"not-a-bool", false},
	} {
		t.Setenv(key, tc.value)
		assert.Equal(t, tc.want, envBool(key), "value %q", tc.value)
	}
}

// TestResolveGates_ManifestMarkerGating exercises the resolveGates helper,
// which is the function serveRun delegates gate resolution to. By calling it
// directly we verify the behavioural contract without needing to run the full
// server (which would swap stdio via ClaimStdio).
func TestResolveGates_ManifestMarkerGating(t *testing.T) {
	// Marker absent: env vars should NOT open gates regardless of what they
	// are set to. The gate vars happen to be unset in this test.
	gates := resolveGates("false", "false", "false", false)
	assert.False(t, gates.WriteAllowed, "write gate closed without marker")
	assert.False(t, gates.AllowAura, "aura gate closed without marker")
	assert.False(t, gates.AllowCredentialWrite, "cred write gate closed without marker")

	// Marker absent: even with gate env vars set, gates stay closed.
	t.Setenv(envMCPAllowWrites, "true")
	t.Setenv(envMCPAllowAura, "true")
	t.Setenv(envMCPAllowCredentialWrite, "true")
	gates = resolveGates("false", "false", "false", false)
	assert.False(t, gates.WriteAllowed, "write gate closed despite env without marker")
	assert.False(t, gates.AllowAura, "aura gate closed despite env without marker")
	assert.False(t, gates.AllowCredentialWrite, "cred write gate closed despite env without marker")
}

// TestResolveGates_MarkerUnlocksEnv verifies that when the manifest marker IS
// present, env-var fallback opens gates.
func TestResolveGates_MarkerUnlocksEnv(t *testing.T) {
	t.Setenv(envMCPAllowWrites, "true")
	t.Setenv(envMCPAllowAura, "true")
	t.Setenv(envMCPAllowCredentialWrite, "true")

	gates := resolveGates("false", "false", "false", true)
	assert.True(t, gates.WriteAllowed, "write gate opened by env with marker")
	assert.True(t, gates.AllowAura, "aura gate opened by env with marker")
	assert.True(t, gates.AllowCredentialWrite, "cred write gate opened by env with marker")
}

// TestResolveGates_MarkerUnsetEnvDefaultsToClosed verifies that when the
// manifest marker is present but no env vars are set, gates stay closed.
func TestResolveGates_MarkerUnsetEnvDefaultsToClosed(t *testing.T) {
	gates := resolveGates("false", "false", "false", true)
	assert.False(t, gates.WriteAllowed, "write gate closed when env unset")
	assert.False(t, gates.AllowAura, "aura gate closed when env unset")
	assert.False(t, gates.AllowCredentialWrite, "cred write gate closed when env unset")
}

// TestResolveGates_FlagsTakePriority verifies that explicit flags (true) are
// authoritative — the env fallback is never consulted when the flag is set.
func TestResolveGates_FlagsTakePriority(t *testing.T) {
	t.Setenv(envMCPAllowWrites, "false")

	// Flag set to true; env is false but should not be consulted.
	gates := resolveGates("true", "false", "false", true)
	assert.True(t, gates.WriteAllowed, "--rw=true must open gate regardless of env")
}

// TestResolveGates_FlagFalseUnsetEnv defaults are safe when both flag and
// env are absent (no marker set).
func TestResolveGates_FlagFalseUnsetEnv(t *testing.T) {
	gates := resolveGates("false", "false", "false", false)
	assert.False(t, gates.WriteAllowed)
	assert.False(t, gates.AllowAura)
	assert.False(t, gates.AllowCredentialWrite)
}

// TestManifestMarkerEnvConstant verifies the marker constant name is stable and
// defaults to absent when not set.
func TestManifestMarkerEnvConstant(t *testing.T) {
	assert.Equal(t, "NEO4J_CLI_MCP_MANIFEST", envManifestMarker,
		"marker constant must match the well-known env var name")
	assert.False(t, envBool(envManifestMarker), "marker must be false when not set")
}

// TestGateEnvVarConstants verifies the gate env-var constant names are stable.
func TestGateEnvVarConstants(t *testing.T) {
	assert.Equal(t, "NEO4J_CLI_MCP_ALLOW_WRITES", envMCPAllowWrites)
	assert.Equal(t, "NEO4J_CLI_MCP_ALLOW_AURA", envMCPAllowAura)
	assert.Equal(t, "NEO4J_CLI_MCP_ALLOW_CREDENTIAL_WRITE", envMCPAllowCredentialWrite)
}
