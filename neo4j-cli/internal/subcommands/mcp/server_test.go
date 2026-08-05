// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupServerTest creates the global state a server needs, plus an executor
// and a server instance, restoring globals on cleanup. Returns the server,
// the client side of an in-memory transport pair, and a cancel function.
func setupServerTest(t *testing.T, gates Gates, defaultFormat string) (*mcpsdk.Server, *mcpsdk.InMemoryTransport, context.Context, context.CancelFunc) {
	t.Helper()

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
	storedRootFactory = serverTestRootFactory

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()
	exec, err := NewExecutor(cfg, serverTestRootFactory)
	require.NoError(t, err)

	server, err := NewServer(cfg, exec, gates, defaultFormat, 8000)
	require.NoError(t, err)

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), serverTestTimeout)

	// Connect the server first; it must be connected before the client
	// (the client sends initialize during its Connect call).
	serverSession, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	return server, clientT, ctx, cancel
}

// serverTestRootFactory returns a minimal stub tree for server tests with
// a docker* subcommand tree, credential, and aura so list_targets works.
func serverTestRootFactory(_ *clicfg.Config) *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
	root.PersistentFlags().String("format", "", "Output format")
	root.PersistentFlags().Bool("rw", false, "Write gate")
	root.PersistentFlags().Bool("debug", false, "Debug mode")

	// docker subcommand
	docker := &cobra.Command{Use: "docker", Short: "Manage Docker containers"}
	docker.AddCommand(&cobra.Command{
		Use: "list", Short: "List Docker containers", RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("container1\n"))
			return nil
		},
	})
	createCmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a container",
		Annotations: map[string]string{"write": "true"},
		RunE: func(c *cobra.Command, _ []string) error {
			name, _ := c.Flags().GetString("name")
			_, _ = c.OutOrStdout().Write([]byte("created: " + name + "\n"))
			return nil
		},
	}
	createCmd.Flags().Bool("no-print-password", false, "Suppress password")
	docker.AddCommand(createCmd)
	docker.AddCommand(&cobra.Command{
		Use: "load", Short: "Load a dataset", RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("loaded\n"))
			return nil
		},
	})
	docker.PersistentFlags().String("name", "", "Container name")
	root.AddCommand(docker)

	// query
	query := &cobra.Command{
		Use:         "query",
		Short:       "Run Cypher queries",
		Annotations: map[string]string{"stdin-reader": "true"},
		RunE: func(c *cobra.Command, _ []string) error {
			return nil
		},
	}
	root.AddCommand(query)

	return root
}

// serverTestTimeout bounds the test so a hang fails the test rather than
// stalling the package.
const serverTestTimeout = 30 * time.Second

// TestServer_ExposesFiveToolsViaInMemoryTransport proves the server registers
// exactly the five neo4j_cli_* tools when exercised through the SDK's in-memory
// transport (REQ-NF-010 — no subprocess, socket or port binding).
func TestServer_ExposesFiveToolsViaInMemoryTransport(t *testing.T) {
	server, clientT, ctx, cancel := setupServerTest(t, Gates{}, "toon")
	defer cancel()
	// Keep server alive until the test is done.
	defer server.RemoveTools()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)

	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, result.Tools, 5, "server must register exactly five tools")

	names := make(map[string]bool, 5)
	for _, tool := range result.Tools {
		names[tool.Name] = true
		assert.Regexp(t, `^neo4j_cli_[a-z0-9_]+$`, tool.Name, "tool name must match naming convention")
	}

	assert.True(t, names["neo4j_cli_list_commands"], "list_commands tool must be registered")
	assert.True(t, names["neo4j_cli_read_docs"], "read_docs tool must be registered")
	assert.True(t, names["neo4j_cli_run"], "run tool must be registered")
	assert.True(t, names["neo4j_cli_run_write"], "run_write tool must be registered")
	assert.True(t, names["neo4j_cli_list_targets"], "list_targets tool must be registered")
}

// TestServer_InstructionsPresent proves the instructions field carries the tree
// index and orientation rules (REQ-F-007).
func TestServer_InstructionsPresent(t *testing.T) {
	text := instructions()
	// Must contain tree index entries
	assert.Contains(t, text, "docker")
	assert.Contains(t, text, "query")
	assert.Contains(t, text, "aura")
	assert.Contains(t, text, "config")
	assert.Contains(t, text, "credential")
	// Must contain orientation rules
	assert.Contains(t, text, "query :schema")
	assert.Contains(t, text, "neo4j_cli_read_docs")
	assert.Contains(t, text, "neo4j_cli_run_write")
	assert.Contains(t, text, "neo4j_cli_list_targets")
	assert.Contains(t, text, "neo4j_cli_list_commands")
}

// TestServer_FormatInjectionToon proves the default format is injected when
// args carry no --format flag.
func TestServer_FormatInjectionToon(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":[]}`),
		},
	}
	modified := injectFormat(req, "toon")
	var args map[string]any
	require.NoError(t, json.Unmarshal(modified.Params.Arguments, &args))
	argList, ok := args["args"].([]any)
	require.True(t, ok)
	require.Contains(t, argList, "--format")
	require.Contains(t, argList, "toon")
}

// TestServer_FormatInjectionJson proves the format injection respects a JSON
// default and does not inject when args already carry --format.
func TestServer_FormatInjectionJson(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":["--format","json"]}`),
		},
	}
	modified := injectFormat(req, "toon")
	require.Same(t, req, modified, "injectFormat must not modify a request that already has --format")
}

// TestServer_FormatInjectionNilArgs proves injection adds --format even when
// args is absent from the request.
func TestServer_FormatInjectionNilArgs(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list"}`),
		},
	}
	modified := injectFormat(req, "toon")
	require.NotSame(t, req, modified, "injectFormat must add --format when args field is absent")
	var args map[string]any
	require.NoError(t, json.Unmarshal(modified.Params.Arguments, &args))
	argList, ok := args["args"].([]any)
	require.True(t, ok)
	require.Contains(t, argList, "--format")
	require.Contains(t, argList, "toon")
}

// TestServer_FormatInjectionJsonEquals proves the format injection respects
// --format=value (equals form) and does not inject.
func TestServer_FormatInjectionJsonEquals(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":["--format=json"]}`),
		},
	}
	modified := injectFormat(req, "toon")
	require.Same(t, req, modified, "injectFormat must not modify when --format=value is present")
}

// TestServer_FormatInjectionPrefixCollision proves --formatting does NOT
// suppress format injection (the earlier HasPrefix would have matched).
func TestServer_FormatInjectionPrefixCollision(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":["--formatting"]}`),
		},
	}
	modified := injectFormat(req, "toon")
	require.NotSame(t, req, modified, "injectFormat must inject when --formatting (not --format) is present")
	var args map[string]any
	require.NoError(t, json.Unmarshal(modified.Params.Arguments, &args))
	argList, ok := args["args"].([]any)
	require.True(t, ok)
	require.Contains(t, argList, "--format")
	require.Contains(t, argList, "toon")
}

// TestServer_FormatInjectionEmptyArgs proves injection works when args is
// present but empty.
func TestServer_FormatInjectionEmptyArgs(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":[]}`),
		},
	}
	modified := injectFormat(req, "json")
	var args map[string]any
	require.NoError(t, json.Unmarshal(modified.Params.Arguments, &args))
	argList, ok := args["args"].([]any)
	require.True(t, ok)
	require.Contains(t, argList, "--format")
	require.Contains(t, argList, "json")
}

// TestServer_CallToolRunInjectsFormat proves that when neo4j_cli_run is called
// through the server, the format is injected and the command executes.
func TestServer_CallToolRunInjectsFormat(t *testing.T) {
	server, clientT, ctx, cancel := setupServerTest(t, Gates{}, "toon")
	defer cancel()
	defer server.RemoveTools()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)

	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "neo4j_cli_run",
		Arguments: map[string]any{
			"command": "docker list",
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "docker list should succeed")
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "container1")
}

// TestServer_CallToolRunWriteWithRw proves the write tool works through the
// transport when WriteAllowed is set and --rw is in args.
func TestServer_CallToolRunWriteWithRw(t *testing.T) {
	server, clientT, ctx, cancel := setupServerTest(t, Gates{WriteAllowed: true}, "toon")
	defer cancel()
	defer server.RemoveTools()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)

	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "neo4j_cli_run_write",
		Arguments: map[string]any{
			"command": "docker create",
			"args":    []any{"--name", "test-container", "--rw"},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError, "docker create with --rw should succeed")
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "created: test-container")
}

// TestServer_CallToolRunWriteRefusedWithoutRw proves the write gate at layer 1
// (WriteAllowed) is enforced through the transport.
func TestServer_CallToolRunWriteRefusedWithoutRw(t *testing.T) {
	server, clientT, ctx, cancel := setupServerTest(t, Gates{WriteAllowed: false}, "toon")
	defer cancel()
	defer server.RemoveTools()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)

	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "neo4j_cli_run_write",
		Arguments: map[string]any{
			"command": "docker create",
		},
	})
	require.NoError(t, err)
	require.True(t, result.IsError, "write without WriteAllowed should fail")
	require.Len(t, result.Content, 1)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "Write operations are not allowed")
}

// TestServe_NotAnnotatedWrite proves serve is NOT annotated write:"true", so
// EnforceWriteGate does not demand --rw merely to start the server.
func TestServe_NotAnnotatedWrite(t *testing.T) {
	root := &cobra.Command{Use: "neo4j-cli"}
	serve := newServeCmd(nil)
	root.AddCommand(serve)

	// serve must not carry annotations: stdout is never a TTY under MCP,
	// so EnforceWriteGate would demand --rw merely to start the server,
	// destroying the read-only default (REQ-NF-006).
	assert.Empty(t, serve.Annotations, "serve must not carry annotations")
}

// ----- No-print-password injection tests -----

func TestServer_NoPrintPasswordInjection(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--name","test"]}`),
		},
	}
	modified := injectFlag(req, shouldInjectNoPrintPassword, "--no-print-password")
	require.NotSame(t, req, modified, "injectFlag must modify docker create request")

	var args map[string]any
	require.NoError(t, json.Unmarshal(modified.Params.Arguments, &args))
	argList, ok := args["args"].([]any)
	require.True(t, ok)
	require.Contains(t, argList, "--no-print-password")
	require.Equal(t, "--no-print-password", argList[0], "--no-print-password must be the first arg")
	require.Contains(t, argList, "--name")
	require.Contains(t, argList, "test")
}

func TestServer_NoPrintPasswordNotInjectedWhenPresent(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--no-print-password","--name","test"]}`),
		},
	}
	modified := injectFlag(req, shouldInjectNoPrintPassword, "--no-print-password")
	require.Same(t, req, modified, "injectFlag must not modify when flag already present")
}

func TestServer_NoPrintPasswordNotInjectedForOtherCommands(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker list","args":[]}`),
		},
	}
	modified := injectFlag(req, shouldInjectNoPrintPassword, "--no-print-password")
	require.Same(t, req, modified, "injectFlag must not modify non-docker-create commands")
}

func TestServer_NoPrintPasswordNotInjectedForEmptyRequest(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{}`),
		},
	}
	modified := injectFlag(req, shouldInjectNoPrintPassword, "--no-print-password")
	require.Same(t, req, modified, "injectFlag must not modify empty request")
}

func TestServer_NoPrintPasswordNotInjectedWhenPresentWithEquals(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"docker create","args":["--no-print-password=true","--name","test"]}`),
		},
	}
	modified := injectFlag(req, shouldInjectNoPrintPassword, "--no-print-password")
	require.Same(t, req, modified, "injectFlag must not modify when --no-print-password=value is present")
}

func TestServer_NoPrintPasswordNotInjectedWhenNoCommand(t *testing.T) {
	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"args":["--name","test"]}`),
		},
	}
	modified := injectFlag(req, shouldInjectNoPrintPassword, "--no-print-password")
	require.Same(t, req, modified, "injectFlag must not modify when no command specified")
}

// ----- Credential store probe tests -----

func TestCheckCredentialStore_KeyringUnavailable(t *testing.T) {
	prevProbe := probeKeyringFn
	t.Cleanup(func() { probeKeyringFn = prevProbe })

	probeKeyringFn = func() error {
		return errors.New("simulated keyring failure")
	}

	fs, err := testfs.GetTestFs(`{"credential-storage":"keyring"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	err = checkCredentialStore(cfg)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "error must be a CLIError")
	require.Equal(t, 1, ce.Code)
	assert.Contains(t, ce.Message, "OS keyring is locked")
	assert.Contains(t, ce.Message, "config set credential-storage insecure")
}

func TestCheckCredentialStore_KeyringOK(t *testing.T) {
	prevProbe := probeKeyringFn
	t.Cleanup(func() { probeKeyringFn = prevProbe })

	probeKeyringFn = func() error {
		return nil
	}

	fs, err := testfs.GetTestFs(`{"credential-storage":"keyring"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	err = checkCredentialStore(cfg)
	require.NoError(t, err)
}

func TestCheckCredentialStore_InsecureModeSkipsProbe(t *testing.T) {
	prevProbe := probeKeyringFn
	t.Cleanup(func() { probeKeyringFn = prevProbe })

	probeKeyringFn = func() error {
		return errors.New("should not be called")
	}

	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	err = checkCredentialStore(cfg)
	require.NoError(t, err, "insecure mode must skip probe")
}
