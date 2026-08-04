// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// The mcp tests live in the EXTERNAL mcp_test package so they can build the
// live tree via app.NewCmd — the only place the `flag.mcp-server` gate is
// applied. app imports this package, but an external test package compiles
// separately, so there is no import cycle (see AGENTS.md).

package mcp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
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
// flag forced on, so the whole-tree policy gate also covers subtrees app.NewCmd
// mounts only behind a flag. It mirrors the identically named helper in
// agentcontext's tests; test helpers cannot be shared across packages, and the
// only alternative — a production helper that builds a fully flag-on tree —
// would be a test-only seam in shipped code.
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

// executableCommands returns every command below root in tree order. Hidden and
// deprecated commands are INCLUDED: help visibility is irrelevant to the policy
// table, since the MCP executor can reach anything cobra can dispatch. Only
// cobra's generated `help` command is skipped.
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

// findCommand resolves a space-separated command path against root, failing the
// test when it does not resolve to exactly that command.
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

// syntheticCmd builds a standalone `neo4j-cli <path...>` chain, for paths the
// live tree cannot supply: `completion` (cobra injects it at Execute() time,
// after the tree the gate walks) and paths that do not exist at all, which is
// how the default-deny branch is reached.
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

// findSubcommand returns the direct subcommand of parent with the given name,
// or nil when it is not registered.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// newExecutor builds an Executor dispatching against trees from newRoot, over
// an in-memory filesystem seeded with telemetry off. Both matter: an OsFs would
// let an executed command reach the developer's real credentials, and a live
// analytics service would let one reach the network.
func newExecutor(t *testing.T, newRoot mcp.RootFactory) *mcp.Executor {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"telemetry": false}`, "{}")
	require.NoError(t, err)
	exec, err := mcp.NewExecutor(clicfg.NewConfig(fs, "test", clicfg.GlobalScope), newRoot)
	require.NoError(t, err)
	return exec
}

// stubFactory adapts a leaf builder into a RootFactory, for behaviours the live
// tree cannot supply (a command that panics). build is called per dispatch
// because a RootFactory must hand out a tree whose flag state is untouched.
func stubFactory(build func() *cobra.Command) mcp.RootFactory {
	return func(*clicfg.Config) *cobra.Command {
		root := &cobra.Command{Use: "neo4j-cli", SilenceErrors: true}
		root.AddCommand(build())
		return root
	}
}

// executeWithin runs one executor call and fails the test rather than hanging
// when it does not come back. A blocked read on stdin is this layer's signature
// failure and a stalled suite diagnoses nothing, so every executor call in these
// tests is bounded — by this helper or by waitWithin.
func executeWithin(t *testing.T, exec *mcp.Executor, args ...string) mcp.CommandResult {
	t.Helper()
	return executeCtxWithin(t, context.Background(), exec, args...)
}

// executeCtxWithin is executeWithin with a caller-supplied context, for the
// cancellation path.
func executeCtxWithin(t *testing.T, ctx context.Context, exec *mcp.Executor, args ...string) mcp.CommandResult {
	t.Helper()
	done := make(chan mcp.CommandResult, 1)
	go func() { done <- exec.Execute(ctx, args) }()
	select {
	case res := <-done:
		return res
	case <-time.After(executorTimeout):
		t.Fatalf("executor did not return within %s for args %v", executorTimeout, args)
		return mcp.CommandResult{}
	}
}

// waitWithin bounds a wait that is not a single executor call, so a deadlock
// fails the test instead of stalling the package until the go-test timeout.
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

// executorTimeout is generous: it is a hang detector, not a latency assertion.
const executorTimeout = 30 * time.Second

// runApp executes args against a freshly built tree, returning stdout, stderr
// and the Execute error.
func runApp(t *testing.T, mcpEnabled bool, args ...string) (stdout, stderr *bytes.Buffer, err error) {
	t.Helper()
	cmd := newAppCmd(t, mcpEnabled)
	cmd.SetArgs(args)
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	err = cmd.Execute()
	return stdout, stderr, err
}

// openTestBundle generates a test .mcpb and returns its zip reader.
func openTestBundle(t *testing.T) (string, *zip.ReadCloser) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.mcpb")
	err := mcp.GenerateBundle(path)
	require.NoError(t, err)
	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	return path, r
}

// readTestManifest generates a test .mcpb and returns the decoded manifest.
func readTestManifest(t *testing.T) (string, map[string]any) {
	t.Helper()
	path, r := openTestBundle(t)
	defer func() { _ = r.Close() }()
	data := readZipFile(t, r, "manifest.json")
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(data, &manifest))
	return path, manifest
}

// readZipFile reads the named entry from a zip reader into memory.
func readZipFile(t *testing.T, r *zip.ReadCloser, name string) []byte {
	t.Helper()
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			require.NoError(t, err)
			data, err := io.ReadAll(rc)
			_ = rc.Close()
			require.NoError(t, err)
			return data
		}
	}
	t.Fatalf("entry %q not found in zip", name)
	return nil
}
