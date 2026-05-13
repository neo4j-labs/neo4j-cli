// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"bytes"
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEnablesTraverseRunHooks(t *testing.T) {
	cobra.EnableTraverseRunHooks = false
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = true })

	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	NewCmd(cfg)

	assert.True(t, cobra.EnableTraverseRunHooks,
		"EnableTraverseRunHooks must be true so PersistentPreRunE hooks on root "+
			"(e.g. format flag binding and write gating) are not shadowed by hooks on child commands")
}

func TestNewCmdRegistersRwFlag(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	cmd := NewCmd(cfg)

	flag := cmd.PersistentFlags().Lookup("rw")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "Allow write operations")
}

// TestNewCmdFlagErrorFuncWrapsAsUsageError asserts cobra's flag-parse errors
// are wrapped into a typed *clierr.CLIError with exit code 2 across both the
// root command and subcommands (cobra walks up to root for FlagErrorFunc).
func TestNewCmdFlagErrorFuncWrapsAsUsageError(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "unknown flag at root", args: []string{"--bad-flag"}},
		{name: "unknown flag on subcommand", args: []string{"aura", "instance", "list", "--bad-flag"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetDefaultTestFs()
			require.NoError(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			cmd := NewCmd(cfg)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			execErr := cmd.Execute()
			require.Error(t, execErr)

			var ce *clierr.CLIError
			require.True(t, errors.As(execErr, &ce), "expected *clierr.CLIError, got %T: %v", execErr, execErr)
			assert.Equal(t, 2, ce.Code)
		})
	}
}
