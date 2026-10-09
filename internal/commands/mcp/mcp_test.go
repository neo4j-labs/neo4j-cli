// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/internal/cli"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPGroup_AbsentWhenFlagDisabled(t *testing.T) {
	root := newAppCmd(t, false)
	assert.Nil(t, findSubcommand(root, "mcp"), "mcp must not be registered with flag.mcp-server off")

	_, _, err := runApp(t, false, "mcp", "tool")
	require.Error(t, err, "invoking mcp with the flag off must fail")
	assert.Contains(t, err.Error(), `unknown command "mcp"`)
}

func TestMCPGroup_PresentWhenFlagEnabled(t *testing.T) {
	root := newAppCmd(t, true)
	group := findSubcommand(root, "mcp")
	require.NotNil(t, group, "mcp must be registered with flag.mcp-server on")

	assert.False(t, group.Hidden, "the feature flag is the only gate; no leaf sets Hidden")
	assert.NotEmpty(t, group.Short)
	assert.NotEmpty(t, group.Long)

	tools := findSubcommand(group, "tool")
	require.NotNil(t, tools, "the tools leaf must be registered")
	assert.False(t, tools.Hidden)

	serve := findSubcommand(group, "serve")
	require.NotNil(t, serve, "the serve leaf must be registered")
	assert.False(t, serve.Hidden)
}

// TestMCPGroup_EnabledByEnvVar covers the override surface CI uses to exercise
// the flag-on path, with no in-process SetForTest involved.
func TestMCPGroup_EnabledByEnvVar(t *testing.T) {
	t.Setenv("NEO4J_CLI_FLAG_MCP_SERVER", "1")
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	root := cli.NewCmd(clicfg.NewConfig(fs, "test"))
	assert.NotNil(t, findSubcommand(root, "mcp"),
		"NEO4J_CLI_FLAG_MCP_SERVER=1 must register the group")
}

// TestMCPGroup_AgentContextReflectsFlag locks the promise that the flag-off tree
// is unchanged for agent-facing consumers: agent-context reflects the live tree,
// and so does the committed skill bundle.
func TestMCPGroup_AgentContextReflectsFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{name: "flag off", enabled: false},
		{name: "flag on", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runApp(t, tc.enabled, "agent-context", "--format", "json")
			require.NoError(t, err, "stderr=%s", stderr.String())

			var envelope struct {
				Commands []struct {
					Path string `json:"path"`
				} `json:"commands"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))

			paths := map[string]bool{}
			for _, c := range envelope.Commands {
				paths[c.Path] = true
			}
			assert.Equal(t, tc.enabled, paths["mcp"], "the mcp command index entry must track the flag")
			if tc.enabled {
				assert.True(t, paths["mcp tool"])
			}
		})
	}
}
