// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// The mcp tests live in the EXTERNAL mcp_test package so they can build the
// live tree via app.NewCmd — the only place the `flag.mcp-server` gate is
// applied. app imports this package, but an external test package compiles
// separately, so there is no import cycle (see AGENTS.md).

package mcp_test

import (
	"bytes"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/app"
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
