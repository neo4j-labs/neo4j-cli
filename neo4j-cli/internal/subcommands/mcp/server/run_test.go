// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

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

	// aura: `api` is the raw HTTP passthrough whose write-ness is resolved from
	// the method at runtime, so it deliberately carries NO write annotation and
	// classifies gated rather than write. `instance list` is a plain read that
	// must stay reachable through neo4j_cli_run.
	aura := &cobra.Command{Use: "aura", Short: "Manage Aura"}
	aura.AddCommand(&cobra.Command{
		Use: "api", Short: "Call the Aura API directly", RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("{}\n"))
			return nil
		},
	})
	instance := &cobra.Command{Use: "instance", Short: "Manage Aura instances"}
	instance.AddCommand(&cobra.Command{
		Use: "list", Short: "List Aura instances", RunE: func(c *cobra.Command, _ []string) error {
			_, _ = c.OutOrStdout().Write([]byte("instance1\n"))
			return nil
		},
	})
	aura.AddCommand(instance)
	root.AddCommand(aura)

	// credential: a write-annotated leaf under the `credential` tree escalates
	// from write to PolicyGatedCredentialWrite via writeGatedPaths.
	credential := &cobra.Command{Use: "credential", Short: "Manage credentials"}
	credential.AddCommand(&cobra.Command{
		Use:         "add",
		Short:       "Add a credential",
		Annotations: map[string]string{"write": "true"},
		RunE:        func(c *cobra.Command, _ []string) error { return nil },
	})
	root.AddCommand(credential)

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

// `aura api` carries no write annotation (its write-ness is the HTTP method, a
// runtime value), so it classifies gated rather than write. neo4j_cli_run
// advertises readOnlyHint, so it must refuse gated commands too or a DELETE
// reaches through it.
func TestHandleRun_GatedCommandRefused(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name    string
		command string
		args    string
	}{
		{name: "aura api is gated, not write-annotated", command: "aura api", args: `,"args":["DELETE","/instances/abc"]`},
		{name: "credential add escalates write to gated", command: "credential add"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			exec := setupRunTest(t)
			// Gates are open: the refusal is the tool's own read/write split,
			// not the --allow-* gate, so it must fire regardless.
			gates := Gates{AllowAura: true, AllowCredentialWrite: true, WriteAllowed: true}

			req := &mcpsdk.CallToolRequest{
				Params: &mcpsdk.CallToolParamsRaw{
					Arguments: json.RawMessage(`{"command":"` + tt.command + `"` + tt.args + `}`),
				},
			}
			result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
			require.NoError(t, err)
			require.True(t, result.IsError)
			tc := result.Content[0].(*mcpsdk.TextContent)
			// Assert the gated message specifically: "is a write command" would
			// also mention neo4j_cli_run_write, and `aura api` must not reach
			// that branch at all (it carries no write annotation).
			assert.Contains(t, tc.Text, "is gated and may mutate state")
			assert.Contains(t, tc.Text, "neo4j_cli_run_write")
		})
	}
}

// The gated refusal must not swallow plain Aura reads: `aura instance list` is
// PolicyAllow and stays reachable through the read-only tool.
func TestHandleRun_AuraReadStillAllowed(t *testing.T) {
	ctx := context.Background()
	exec := setupRunTest(t)
	gates := Gates{}

	req := &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Arguments: json.RawMessage(`{"command":"aura instance list"}`),
		},
	}
	result, err := HandleRun(ctx, req, exec, gates, runTestRootFactory)
	require.NoError(t, err)
	require.False(t, result.IsError)
	tc := result.Content[0].(*mcpsdk.TextContent)
	assert.Contains(t, tc.Text, "instance1")
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

// TestResolveCommand_RejectsNonRunnableParent guards the High-severity policy
// bypass found by the security review: Find returns a non-runnable PARENT for a
// partial path, and parents report IsAvailableCommand()==true. Classification
// would then run against the parent (allow) while execArgs routes cobra to a
// child leaf, so `command:"config" args:["set","credential-storage",…]` slipped
// past the deny rule on `config set`.
func TestResolveCommand_RejectsNonRunnableParent(t *testing.T) {
	root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
	root.PersistentFlags().String("format", "", "")
	root.PersistentFlags().Bool("rw", false, "")
	parent := &cobra.Command{Use: "config"}
	leaf := &cobra.Command{
		Use:         "set",
		Annotations: map[string]string{"write": "true"},
		RunE:        func(*cobra.Command, []string) error { return nil },
	}
	parent.AddCommand(leaf)
	root.AddCommand(parent)

	// Sanity: cobra really does hand back the parent and call it available.
	found, _, err := root.Find([]string{"config"})
	require.NoError(t, err)
	require.False(t, found.Runnable(), "parent must be non-runnable for this test to mean anything")
	require.True(t, found.IsAvailableCommand(), "parents report available, which is why Runnable() is needed")

	raw, err := json.Marshal(map[string]any{
		"command": "config",
		"args":    []any{"set", "credential-storage", "insecure"},
	})
	require.NoError(t, err)

	r := resolveCommand(context.Background(),
		&mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{Arguments: raw}},
		nil, Gates{}, func(*clicfg.Config) *cobra.Command { return root })

	require.NotNil(t, r.err, "a non-runnable parent path must be refused")
	require.True(t, r.err.IsError, "the refusal must be a tool error")
	require.NotEmpty(t, r.err.Content)
	text, ok := r.err.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "Unknown command",
		"parent path must be rejected as unknown, not classified against the parent")
}
