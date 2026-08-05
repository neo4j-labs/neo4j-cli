// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRunTest(t *testing.T) *Executor {
	t.Helper()
	// Ensure the global version is set since HandleRun uses storedVersion to
	// build trees for command resolution. Save/restore so we don't interfere
	// with other internal tests (e.g. list_commands) that also depend on them.
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
	// The root factory is passed explicitly to HandleRun from tests so each
	// test can control which tree is used without relying on the global.
	// We leave storedRootFactory nil to prevent accidental cross-talk with
	// list_commands tests that depend on it.
	storedRootFactory = nil

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()
	exec, err := NewExecutor(cfg, runTestRootFactory)
	require.NoError(t, err)
	return exec
}

// runTestRootFactory returns a stub cobra tree for testing the run handler
// without importing package app.
func runTestRootFactory(cfg *clicfg.Config) *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}

	// docker subcommands: list (read), load (read)
	docker := &cobra.Command{Use: "docker", Short: "Manage Docker containers"}
	docker.AddCommand(&cobra.Command{
		Use: "list", Short: "List Docker containers", RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("container1\ncontainer2\n"))
			return nil
		},
	})
	docker.AddCommand(&cobra.Command{
		Use: "load", Short: "Load a dataset into a container", RunE: func(c *cobra.Command, _ []string) error {
			name, _ := c.Flags().GetString("name")
			_, _ = c.OutOrStdout().Write([]byte("loaded: " + name + "\n"))
			return nil
		},
	})
	docker.PersistentFlags().String("name", "", "Container name")
	root.AddCommand(docker)

	// query: reads os.Stdin when no positional given
	query := &cobra.Command{
		Use:         "query",
		Short:       "Run Cypher queries",
		Annotations: map[string]string{"stdin-reader": "true"},
		RunE: func(c *cobra.Command, _ []string) error {
			return errors.New("not implemented in test stub")
		},
	}
	root.AddCommand(query)

	// Write-annotated leaf (simulates "docker create")
	create := &cobra.Command{
		Use:         "create",
		Short:       "Create a Docker container",
		Annotations: map[string]string{"write": "true"},
		RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("created\n"))
			return nil
		},
	}
	docker.AddCommand(create)
	create.Flags().String("name", "", "Container name")

	// config with a set subcommand (needs a write annotation for its key=value args)
	config := &cobra.Command{Use: "config", Short: "Manage configuration"}
	config.AddCommand(&cobra.Command{
		Use: "get", Short: "Get a config value", RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("value\n"))
			return nil
		},
	})
	config.AddCommand(&cobra.Command{
		Use:         "set",
		Short:       "Set a config value",
		Annotations: map[string]string{"write": "true"},
		RunE:        func(c *cobra.Command, _ []string) error { return nil },
	})
	root.AddCommand(config)

	return root
}

func TestHandleRun_MissingCommand(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{}}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Missing required argument: command")
}

func TestHandleRun_UnknownCommand(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"nonexistent"}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Unknown command")
}

func TestHandleRun_WriteCommandRefused(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	// docker create is write-annotated in the stub
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create"}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "is a write command")
	assert.Contains(t, tc.Text, "neo4j_cli_run_write")
}

func TestHandleRun_RejectsRwInArgs(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","args":["--rw"]}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "--rw")
	assert.Contains(t, tc.Text, "neo4j_cli_run_write")
}

func TestHandleRun_RejectsDebugInArgs(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","args":["--debug"]}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "--debug")
	assert.Contains(t, tc.Text, "not accepted")
}

func TestHandleRun_UnknownFlagWithDidYouMean(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	// docker load has a --name flag; --nme is close
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","args":["--nme","mycontainer"]}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "unknown flag --nme")
	assert.Contains(t, tc.Text, "did you mean")
	assert.Contains(t, tc.Text, "--name")
}

func TestHandleRun_KnownFlagPasses(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	// docker load with a valid --name flag and value
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","args":["--name","test-container"]}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.False(t, result.IsError, "known flag should not produce an error")
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "loaded: test-container")
}

func TestHandleRun_ArgsCap(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	// Build args array with MaxRunArgs + 1 items
	args := make([]any, MaxRunArgs+1)
	for i := range args {
		args[i] = "x"
	}
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":` + string(argsJSON) + `}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "exceeds maximum")
}

func TestHandleRun_QueryWithoutArgsIsRefused(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"query"}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "requires a positional argument")
}

func TestHandleRun_Success(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list"}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "container1")
}

func TestHandleRun_QueryWithArgsAllowed(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	// query with a positional arg should pass validation (though query stub returns error)
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"query","args":["RETURN 1"]}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	// The stub command for query returns an error, but that's a runtime error
	// after validation, not a pre-exec validation error.
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.NotContains(t, tc.Text, "requires a positional argument",
		"a query with a positional arg must pass stdin validation")
}

func TestHandleRun_WriteCommandInArgsQuery(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	// query --rw should be classified as write by the policy table (writeArgs rule)
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"query","args":["--rw","DETACH DELETE n"]}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	// The write classification (writeArgs rule for query --rw) catches
	// it before the --rw rejection check (Classify fires first). Error
	// names the write tool.
	assert.Contains(t, tc.Text, "write command")
	assert.Contains(t, tc.Text, "neo4j_cli_run_write")
}

// Test editDistance directly.
func TestEditDistance(t *testing.T) {
	assert.Equal(t, 0, editDistance("", ""))
	assert.Equal(t, 0, editDistance("name", "name"))
	assert.Equal(t, 1, editDistance("name", "nam"))
	assert.Equal(t, 1, editDistance("nme", "name"))
	assert.Equal(t, 2, editDistance("na", "name"))
	assert.Equal(t, 1, editDistance("", "a"))
}

func TestClosestFlag(t *testing.T) {
	known := map[string]bool{"name": true, "format": true, "wait": true, "debug": true, "rw": true}

	assert.Equal(t, "name", closestFlag("nme", known))
	assert.Equal(t, "format", closestFlag("frmat", known))
	assert.Equal(t, "wait", closestFlag("wiat", known))
	// Best distance is 5 which is > threshold of 3; all flags rejected.
	assert.Equal(t, "", closestFlag("zzzzz", known), "too distant")
}

func TestKnownFlagNames(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "name flag")
	cmd.Flags().Bool("verbose", false, "verbose")
	cmd.Flags().Bool("hidden-flag", false, "hidden")
	// MarkHidden marks the flag with given name hidden.
	require.NoError(t, cmd.Flags().MarkHidden("hidden-flag"))

	names := knownFlagNames(cmd)
	assert.True(t, names["name"])
	assert.True(t, names["verbose"])
	assert.False(t, names["hidden-flag"], "hidden flags must be excluded")
}

func TestIsStdinLeaf(t *testing.T) {
	query := &cobra.Command{
		Use:         "query",
		Annotations: map[string]string{"stdin-reader": "true"},
	}
	docker := &cobra.Command{Use: "docker"}

	// Wrap in roots to give them proper paths
	root := &cobra.Command{Use: "neo4j-cli"}
	root.AddCommand(query)
	root.AddCommand(docker)

	assert.True(t, isStdinLeaf(query))
	assert.False(t, isStdinLeaf(docker))
}

func TestParseRunArgs_NoArgs(t *testing.T) {
	command, args := parseRunArgs(&mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list"}`),
		},
	})
	assert.Equal(t, "docker list", command)
	assert.Empty(t, args)
}

func TestParseRunArgs_WithArgs(t *testing.T) {
	command, args := parseRunArgs(&mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker load","args":["--name","test"]}`),
		},
	})
	assert.Equal(t, "docker load", command)
	assert.Equal(t, []string{"--name", "test"}, args)
}

func TestParseRunArgs_NilRequest(t *testing.T) {
	command, args := parseRunArgs(nil)
	assert.Empty(t, command)
	assert.Empty(t, args)
}

func TestParseRunArgs_NonStringArgs(t *testing.T) {
	command, args := parseRunArgs(&mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"test","args":[1,2,3]}`),
		},
	})
	assert.Equal(t, "test", command)
	assert.Empty(t, args, "non-string items must be skipped")
}
