// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeGateRootFactory returns a stub tree with EnforceWriteGate installed on the
// root, used by the authoritative gate test to prove layer 3 catches writes even
// when the MCP-layer gates are open. Commands are nested under a `docker` parent
// so the policy table classifies them (docker is in exposedPaths).
func writeGateRootFactory(cfg *clicfg.Config) *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
	root.PersistentFlags().Bool("rw", false, "Allow write operations")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return flags.EnforceWriteGate(cmd)
	}

	docker := &cobra.Command{Use: "docker", Short: "Docker"}
	create := &cobra.Command{
		Use:         "create",
		Short:       "Create",
		Annotations: map[string]string{"write": "true"},
		RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("created\n"))
			return nil
		},
	}
	create.Flags().String("name", "", "Container name")
	docker.AddCommand(create)
	root.AddCommand(docker)

	return root
}

func TestHandleRunWrite_MissingCommand(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{}}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Missing required argument: command")
}

func TestHandleRunWrite_UnknownCommand(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"nonexistent"}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Unknown command")
}

func TestHandleRunWrite_WriteGateClosed(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)

	// Gate 1: WriteAllowed=false — layer 1 refuses before any tree is built.
	gates := Gates{WriteAllowed: false}
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create"}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Write operations are not allowed")
	assert.Contains(t, tc.Text, "mcp serve --rw")
}

func TestHandleRunWrite_ReadCommandRefused(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	// docker list is an allow-classified command (no write annotation).
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list"}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "is a read-only command")
	assert.Contains(t, tc.Text, "neo4j_cli_run")
}

func TestHandleRunWrite_WriteCommandSucceeds(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create"}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "created")
}

func TestHandleRunWrite_RejectsDebugInArgs(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--debug"]}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "--debug")
	assert.Contains(t, tc.Text, "not accepted")
}

func TestHandleRunWrite_UnknownFlagWithDidYouMean(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--nme","mycontainer"]}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "unknown flag --nme")
	assert.Contains(t, tc.Text, "did you mean")
	assert.Contains(t, tc.Text, "--name")
}

func TestHandleRunWrite_ArgsCap(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	args := make([]any, MaxRunArgs+1)
	for i := range args {
		args[i] = "x"
	}
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":` + string(argsJSON) + `}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "exceeds maximum")
}

func TestHandleRunWrite_QueryWithoutRwIsReadOnly(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	// query without --rw is classified as PolicyAllow (read-only), so the write
	// tool refutes it and points at neo4j_cli_run. The stdin guard would fire
	// after the classify check only for write-classified commands.
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"query"}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "is a read-only command")
	assert.Contains(t, tc.Text, "neo4j_cli_run")
}

func TestHandleRunWrite_KnownFlagPasses(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--name","test-container"]}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.False(t, result.IsError, "known flag should not produce an error")
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "created")
}

// TestHandleRunWrite_AuthoritativeGate proves that even with MCP-level layers 1
// and 2 both open (WriteAllowed=true, no gated flags needed), layer 3
// (flags.EnforceWriteGate inside Execute) still catches a write attempt that
// lacks --rw. This is the test that proves the architecture: a bug in the MCP
// layer cannot produce an unflagged write.
func TestHandleRunWrite_AuthoritativeGate(t *testing.T) {
	ctx := context.Background()

	// Build a separate executor and use writeGateRootFactory which has
	// EnforceWriteGate installed on the root.
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()
	exec, err := NewExecutor(cfg, writeGateRootFactory)
	require.NoError(t, err)

	// All MCP-layer gates are open: WriteAllowed=true and no gated flags needed.
	gates := Gates{WriteAllowed: true}

	// "docker create" is write-annotated but args contain NO --rw.
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create"}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, writeGateRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError, "the authoritative gate must catch a write without --rw")

	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "this command writes")
	assert.Contains(t, tc.Text, "pass --rw")
}

// TestHandleRunWrite_WriteWithRwPassesAllGates proves that when --rw IS in args,
// all three gates pass and the command produces the expected output.
func TestHandleRunWrite_WriteWithRwPassesAllGates(t *testing.T) {
	ctx := context.Background()

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()
	exec, err := NewExecutor(cfg, writeGateRootFactory)
	require.NoError(t, err)

	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--rw"]}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, writeGateRootFactory)
	require.NoError(t, err)
	require.False(t, result.IsError, "write with --rw should pass all three gates")
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "created")
}

// TestHandleRunWrite_DenyClassifiedRefused proves that a deny-classified path
// is refused at layer 2 even when the write-allowed gate is open.
func TestHandleRunWrite_DenyClassifiedRefused(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	// config set with credential-storage is deny-classified via deniedArgs.
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"config set","args":["credential-storage","insecure"]}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "cannot be run over MCP")
}

// TestHandleRunWrite_QueryWithRwPassesStdinGuard proves that query --rw with a
// positional arg passes the stdin guard and routes to execution. The stub stub
// query command returns an error, but it's a runtime execution error, not a
// pre-exec validation error.
func TestHandleRunWrite_QueryWithRwPassesStdinGuard(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{WriteAllowed: true}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"query","args":["--rw","RETURN 1"]}`),
		},
	}
	result, err := HandleRunWrite(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	// The stub query command returns an error, but that's a runtime error
	// after validation, not a pre-exec validation error.
	require.True(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.NotContains(t, tc.Text, "requires a positional argument",
		"a query with a positional arg must pass stdin validation")
}
