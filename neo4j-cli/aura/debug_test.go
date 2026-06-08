// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"bytes"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugFlagRegisteredAndInherited(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewCmd(cfg)

	root := cmd.PersistentFlags().Lookup("debug")
	require.NotNil(t, root, "--debug must be registered on the aura root")
	assert.Equal(t, "false", root.DefValue)
	assert.Contains(t, root.Usage, "NEO4J_DEBUG")
	assert.Contains(t, root.Usage, "stderr")

	// Inherited by a subcommand via persistent-flag merging.
	instance, _, err := cmd.Find([]string{"instance", "list"})
	require.NoError(t, err)
	require.NoError(t, instance.ParseFlags(nil))
	assert.NotNil(t, instance.Flag("debug"), "--debug must be inherited by aura subcommands")
}

// runDebugResolution mounts the aura tree under a stand-in neo4j-cli root with
// EnableTraverseRunHooks=true (mirroring the shipped surface) so the aura-root
// PersistentPreRunE runs even when a subcommand defines its own. It drives the
// resolution to the point the subcommand RunE would execute, then returns the
// resolved debug state stored on cfg.
func runDebugResolution(t *testing.T, args []string) bool {
	t.Helper()

	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	captured := false
	auraCmd := NewCmd(cfg)
	auraCmd.Use = "aura"

	// Replace the leaf RunE so we don't hit the network; capture the resolved
	// debug state after all PersistentPreRunE hooks have run.
	leaf, _, err := auraCmd.Find([]string{"instance", "list"})
	require.NoError(t, err)
	leaf.RunE = func(_ *cobra.Command, _ []string) error {
		captured = cfg.Aura.Debug()
		return nil
	}

	root := &cobra.Command{Use: "neo4j-cli"}
	root.AddCommand(auraCmd)
	prev := cobra.EnableTraverseRunHooks
	cobra.EnableTraverseRunHooks = true
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = prev })

	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"aura", "instance", "list"}, args...))
	require.NoError(t, root.Execute())

	return captured
}

func TestDebugResolvedOnMountedSurface(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		envValue  string
		wantDebug bool
	}{
		{name: "default off", args: nil, wantDebug: false},
		{name: "flag on", args: []string{"--debug"}, wantDebug: true},
		{name: "env=1 on", args: nil, envValue: "1", wantDebug: true},
		{name: "env=true leaves off", args: nil, envValue: "true", wantDebug: false},
		{name: "explicit --debug=false overrides env=1", args: []string{"--debug=false"}, envValue: "1", wantDebug: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue != "" {
				t.Setenv("NEO4J_DEBUG", tc.envValue)
			}
			assert.Equal(t, tc.wantDebug, runDebugResolution(t, tc.args))
		})
	}
}
