// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"bytes"
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStandaloneCmdRegistersRwFlag(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewStandaloneCmd(cfg)

	flag := cmd.PersistentFlags().Lookup("rw")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "Allow write operations")
}

func TestNewCmdDoesNotRegisterRwFlag(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewCmd(cfg)

	assert.Nil(t, cmd.PersistentFlags().Lookup("rw"))
}

// TestNewStandaloneCmdFlagErrorFuncWrapsAsUsageError asserts cobra's
// flag-parse errors on the standalone aura root and its subcommands are
// wrapped into *clierr.CLIError with exit code 2.
func TestNewStandaloneCmdFlagErrorFuncWrapsAsUsageError(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "unknown flag at root", args: []string{"--bad-flag"}},
		{name: "unknown flag on subcommand", args: []string{"instance", "list", "--bad-flag"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetDefaultTestFs()
			require.NoError(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
			cmd := NewStandaloneCmd(cfg)
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
