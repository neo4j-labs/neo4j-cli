// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// This file is in the INTERNAL mcp package (unlike most tests) so it can
// exercise the global-state-backed tool handler directly. It does NOT import
// package app, which would create an import cycle (app imports mcp).

package server

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleListCommands_NoArg(t *testing.T) {
	setupListCommandsTest(t)
	req := &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{}}
	result, err := HandleListCommands(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError, "no-arg must not be an error")
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	require.NotEmpty(t, text.Text)

	assert.Contains(t, text.Text, "docker —")
	assert.Contains(t, text.Text, "query —")
	// Never the full agent-context envelope
	assert.NotContains(t, text.Text, "exit_codes")
	assert.NotContains(t, text.Text, "schema_version")
	assert.NotContains(t, text.Text, "cli_version")
	// completion is explicitly filtered
	assert.NotContains(t, text.Text, "completion")
	// Output must be the compact tree index, not the full tree
	assert.Less(t, len(text.Text), 2000, "no-arg output must be the compact tree index")
}

func TestHandleListCommands_KnownTree(t *testing.T) {
	setupListCommandsTest(t)

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"tree":"docker"}`),
		},
	}
	result, err := HandleListCommands(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	require.NotEmpty(t, text.Text)

	assert.Contains(t, text.Text, "docker —")
	assert.Contains(t, text.Text, "docker create —")
	assert.Contains(t, text.Text, "docker load —")
	// Never the full envelope
	assert.NotContains(t, text.Text, "exit_codes")
}

func TestHandleListCommands_UnknownTree(t *testing.T) {
	setupListCommandsTest(t)

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"tree":"nonexistent"}`),
		},
	}
	result, err := HandleListCommands(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "unknown tree must be an error")
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "Unknown tree")
	assert.Contains(t, text.Text, "nonexistent")
}

func TestHandleListCommands_FiltersCompletion(t *testing.T) {
	setupListCommandsTest(t)

	// Verify completion is not in the tree index — it should be removed by the
	// handler even if the stub tree carried it.
	for _, name := range toolTreeNames {
		assert.NotEqual(t, "completion", name, "toolTreeNames must not contain completion")
	}

	// Also verify none of the tree expansions contain a completion node.
	for _, name := range toolTreeNames {
		req := &mcpsdk.CallToolRequest{
			Params: &mcpsdk.CallToolParamsRaw{
				Arguments: func() json.RawMessage {
					data, _ := json.Marshal(map[string]any{"tree": name})
					return data
				}(),
			},
		}
		result, err := HandleListCommands(context.Background(), req)
		require.NoError(t, err)
		if result.IsError {
			continue
		}
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcpsdk.TextContent); ok {
				assert.NotContains(t, tc.Text, "completion", "tree %q", name)
			}
		}
	}
}

// testCmdByName finds the direct child of parent with the given name.
func testCmdByName(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// stubCmd returns a cobra command that is AvailableCommand (runnable with no
// side effects), for stub trees.
func stubCmd(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Run: func(*cobra.Command, []string) {}}
}

// testRootFactory returns a stub cobra tree for testing the list_commands
// handler without importing package app (which would create an import cycle).
func testRootFactory(cfg *clicfg.Config) *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli", Short: "root"}
	root.AddCommand(stubCmd("docker", "Manage Docker containers"))
	root.AddCommand(stubCmd("aura", "Manage Aura instances"))
	root.AddCommand(stubCmd("query", "Run Cypher queries"))
	root.AddCommand(stubCmd("config", "Manage configuration"))
	docker := testCmdByName(root, "docker")
	docker.AddCommand(stubCmd("create", "Create a Docker container"))
	docker.AddCommand(stubCmd("list", "List Docker containers"))
	docker.AddCommand(stubCmd("load", "Load a dataset into a container"))
	docker.AddCommand(stubCmd("start", "Start a container"))
	docker.AddCommand(stubCmd("stop", "Stop a container"))
	return root
}

// setupListCommandsTest populates the package-level globals the tool handler
// depends on, using a stub tree to avoid an import cycle with package app.
// Saves and restores globals so independent internal test suites do not
// interfere with each other.
func setupListCommandsTest(t *testing.T) {
	t.Helper()
	if storedRootFactory != nil {
		return
	}
	prevVersion := storedVersion
	prevFlagStates := storedFlagStates
	prevRootFactory := storedRootFactory
	t.Cleanup(func() {
		storedVersion = prevVersion
		storedFlagStates = prevFlagStates
		storedRootFactory = prevRootFactory
	})

	storedVersion = "test"
	storedFlagStates = map[string]bool{}
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()
	storedRootFactory = testRootFactory
	root := testRootFactory(cfg)
	for _, sub := range root.Commands() {
		// IsAvailableCommand would reject help; our stub has no help so all pass.
		if !sub.IsAvailableCommand() {
			continue
		}
		toolTreeNames = append(toolTreeNames, sub.Name())
	}
}
