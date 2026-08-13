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
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

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

// newMCPInstallFixture builds the live tree over a MemMapFs with the Claude
// Desktop detect dir seeded (when detected=true) and HOME set, so install
// tests can exercise agent detection and config writes without touching disk.
func newMCPInstallFixture(t *testing.T, detected bool) (*bytes.Buffer, *bytes.Buffer, *cobra.Command) {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	if detected {
		t.Setenv("HOME", "/Users/test")
		claudeDir := "/Users/test/Library/Application Support/Claude"
		require.NoError(t, fs.MkdirAll(claudeDir, 0755))
	}

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	for name := range clicfg.Registry {
		cfg.Flags.SetForTest(name, true)
	}
	root := app.NewCmd(cfg)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)

	return stdout, stderr, root
}

// runMCPApp is like runApp but uses the MCP-install fixture if detected.
func runMCPApp(t *testing.T, detected bool, args ...string) (stdout, stderr *bytes.Buffer, err error) {
	t.Helper()
	stdout, stderr, root := newMCPInstallFixture(t, detected)
	root.SetArgs(args)
	err = root.Execute()
	return stdout, stderr, err
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
