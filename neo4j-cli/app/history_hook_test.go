// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/history"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gokeyring "github.com/zalando/go-keyring"
)

func TestShouldRecordHistory(t *testing.T) {
	completeCmd := &cobra.Command{Use: cobra.ShellCompRequestCmd}
	helpCmd := &cobra.Command{Use: "help"}

	historyParent := &cobra.Command{Use: "history"}
	historyLeaf := &cobra.Command{Use: "list"}
	historyParent.AddCommand(historyLeaf)

	auraParent := &cobra.Command{Use: "aura"}
	instanceParent := &cobra.Command{Use: "instance"}
	instanceLeaf := &cobra.Command{Use: "list"}
	auraParent.AddCommand(instanceParent)
	instanceParent.AddCommand(instanceLeaf)

	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{name: "completion command", cmd: completeCmd, want: false},
		{name: "help command", cmd: helpCmd, want: false},
		{name: "history parent", cmd: historyParent, want: false},
		{name: "history leaf", cmd: historyLeaf, want: false},
		{name: "nested aura leaf", cmd: instanceLeaf, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldRecordHistory(tc.cmd))
		})
	}
}

// TestHistoryHookRecordsThroughExecute drives the full command via Execute so
// the real root PersistentPreRunE (format binding, credential init, history
// recording) runs end-to-end and a history entry lands on disk. `config get`
// is a harmless local read that resolves the format flag normally.
func TestHistoryHookRecordsThroughExecute(t *testing.T) {
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs(`{"history-enabled":true,"history-limit":1000}`, "{}")
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test-version", clicfg.GlobalScope)
	root := NewCmd(cfg)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"config", "get", "format"})

	require.NoError(t, root.Execute())

	entries, err := history.Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "test-version", entries[0].Version)
}

// TestHistoryHookFiresForNestedAuraSubcommand verifies recording fires for an
// aura subcommand whose subtree defines its own PersistentPreRunE. With
// EnableTraverseRunHooks=true the root hook runs pre-run alongside the child
// hook, so the entry is written even though the leaf's RunE later fails (no
// network/credentials).
func TestHistoryHookFiresForNestedAuraSubcommand(t *testing.T) {
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs(`{"history-enabled":true,"history-limit":1000}`, "{}")
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test-version", clicfg.GlobalScope)
	root := NewCmd(cfg)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"aura", "workspace", "list"})

	// Execute likely errors at the leaf (no creds), but recording is pre-run.
	_ = root.Execute()

	entries, err := history.Load(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "test-version", entries[0].Version)
}

// TestHistoryHookSkipsHelpCommandThroughExecute verifies that running the help
// flow does not write a history entry — the excluded `help` command's name is
// detected via cmd inspection by the root hook.
func TestHistoryHookSkipsHelpCommandThroughExecute(t *testing.T) {
	gokeyring.MockInit()

	fs, err := testfs.GetTestFs(`{"history-enabled":true,"history-limit":1000}`, "{}")
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test-version", clicfg.GlobalScope)
	root := NewCmd(cfg)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"help"})

	require.NoError(t, root.Execute())

	entries, err := history.Load(cfg)
	require.NoError(t, err)
	assert.Empty(t, entries)

	_, statErr := fs.Stat(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "history.jsonl"))
	assert.True(t, statErr != nil)
}
