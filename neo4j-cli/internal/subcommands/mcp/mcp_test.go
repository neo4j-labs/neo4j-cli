// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPGroup_AbsentWhenFlagDisabled(t *testing.T) {
	root := newAppCmd(t, false)
	assert.Nil(t, findSubcommand(root, "mcp"), "mcp must not be registered with flag.mcp-server off")

	_, _, err := runApp(t, false, "mcp", "tools")
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

	tools := findSubcommand(group, "tools")
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
	root := app.NewCmd(clicfg.NewConfig(fs, "test", clicfg.GlobalScope))
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
				Commands map[string]struct {
					Subcommands map[string]any `json:"subcommands"`
				} `json:"commands"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))

			group, ok := envelope.Commands["mcp"]
			assert.Equal(t, tc.enabled, ok, "commands.mcp presence must track the flag")
			if tc.enabled {
				assert.Contains(t, group.Subcommands, "tools")
			}
		})
	}
}
