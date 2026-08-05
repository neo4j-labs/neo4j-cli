// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
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
		{AllowAuraFlag, "false"},
		{AllowCredentialWriteFlag, "false"},
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
