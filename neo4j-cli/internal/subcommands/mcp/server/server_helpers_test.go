// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// newAppCmd builds the live neo4j-cli tree over an in-memory filesystem, with
// the mcp feature flag forced to mcpEnabled.
func newAppCmd(t *testing.T, mcpEnabled bool) *cobra.Command {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	cfg.Flags.SetForTest("flag.mcp-server", mcpEnabled)
	return app.NewCmd(cfg)
}

// newAppCmdEveryFlagEnabled builds the live tree with EVERY registered feature
// flag forced on.
func newAppCmdEveryFlagEnabled(t *testing.T) *cobra.Command {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	for name := range clicfg.Registry {
		cfg.Flags.SetForTest(name, true)
	}
	return app.NewCmd(cfg)
}

// executableCommands returns every command below root in tree order.
func executableCommands(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			if sub.Name() == "help" {
				continue
			}
			out = append(out, sub)
			walk(sub)
		}
	}
	walk(root)
	return out
}

// findCommand resolves a space-separated command path against root.
func findCommand(t *testing.T, root *cobra.Command, path string) *cobra.Command {
	t.Helper()
	tokens := strings.Fields(path)
	cmd, _, err := root.Find(tokens)
	require.NoError(t, err, "resolving %q", path)
	require.Equal(t, path, strings.Join(commandPathOf(cmd), " "), "%q did not resolve to itself", path)
	return cmd
}

// commandPathOf mirrors the production path derivation for assertion purposes.
func commandPathOf(cmd *cobra.Command) []string {
	var path []string
	for c := cmd; c != nil && c.Parent() != nil; c = c.Parent() {
		path = append([]string{c.Name()}, path...)
	}
	return path
}

// syntheticCmd builds a standalone `neo4j-cli <path...>` chain.
func syntheticCmd(write bool, path ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "neo4j-cli"}
	for _, name := range path {
		child := &cobra.Command{Use: name}
		cmd.AddCommand(child)
		cmd = child
	}
	if write {
		cmd.Annotations = map[string]string{"write": "true"}
	}
	return cmd
}

// findSubcommand returns the direct subcommand of parent with the given name.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// newExecutor builds an Executor dispatching against trees from newRoot, over
// an in-memory filesystem seeded with telemetry off.
func newExecutor(t *testing.T, newRoot server.RootFactory) *server.Executor {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"telemetry": false}`, "{}")
	require.NoError(t, err)
	exec, err := server.NewExecutor(clicfg.NewConfig(fs, "test", clicfg.GlobalScope), newRoot)
	require.NoError(t, err)
	return exec
}

// stubFactory adapts a leaf builder into a RootFactory, for behaviours the live
// tree cannot supply.
func stubFactory(build func() *cobra.Command) server.RootFactory {
	return func(*clicfg.Config) *cobra.Command {
		root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
		root.AddCommand(build())
		return root
	}
}

// executeWithin runs one executor call and fails the test rather than hanging
// when it does not come back.
func executeWithin(t *testing.T, exec *server.Executor, args ...string) server.CommandResult {
	t.Helper()
	return executeCtxWithin(t, context.Background(), exec, args...)
}

// executeCtxWithin is executeWithin with a caller-supplied context.
func executeCtxWithin(t *testing.T, ctx context.Context, exec *server.Executor, args ...string) server.CommandResult {
	t.Helper()
	done := make(chan server.CommandResult, 1)
	go func() { done <- exec.Execute(ctx, args) }()
	select {
	case res := <-done:
		return res
	case <-time.After(executorTimeout):
		t.Fatalf("executor did not return within %s for args %v", executorTimeout, args)
		return server.CommandResult{}
	}
}

// waitWithin bounds a wait that is not a single executor call.
func waitWithin(t *testing.T, what string, wait func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(executorTimeout):
		t.Fatalf("%s did not finish within %s", what, executorTimeout)
	}
}

// executorTimeout is generous: it is a hang detector.
const executorTimeout = 30 * time.Second

// initCheck ensures cobra.EnableTraverseRunHooks is set on the test tree,
// mirroring neo4j-cli's root (so the mcp group's PersistentPreRunE fires).
var _ = cobra.EnableTraverseRunHooks

// The unused import of the mcp package is intentional: it ensures the
// RootFactory type alias and any other re-exports resolve at import time,
// rather than hiding a wiring issue until a deep-path test.
var _ = mcp.NewCmd
