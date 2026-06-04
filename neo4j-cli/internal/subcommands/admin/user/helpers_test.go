// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptPassword_FlagSet_ReturnsFlagValue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("password", "", "password flag")
	require.NoError(t, cmd.Flags().Set("password", "secret123"))

	pw, err := promptPassword(cmd)
	require.NoError(t, err)
	assert.Equal(t, "secret123", pw)
}

func TestPromptPassword_NoFlagNoTTY_ReturnsUsageError(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	cmd := &cobra.Command{}
	cmd.Flags().String("password", "", "password flag")
	errBuf := bytes.NewBuffer(nil)
	cmd.SetErr(errBuf)

	_, err := promptPassword(cmd)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code) // usage_error
}

func TestPromptPassword_TTY_ReadsFromPasswordReader(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	origReader := passwordReader
	passwordReader = func() (string, error) { return "fromtty", nil }
	t.Cleanup(func() { passwordReader = origReader })

	errBuf := bytes.NewBuffer(nil)
	cmd := &cobra.Command{}
	cmd.Flags().String("password", "", "password flag")
	cmd.SetErr(errBuf)

	pw, err := promptPassword(cmd)
	require.NoError(t, err)
	assert.Equal(t, "fromtty", pw)
}

func TestPromptPassword_TTY_ReaderError_PropagatesError(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	origReader := passwordReader
	passwordReader = func() (string, error) { return "", errors.New("read error") }
	t.Cleanup(func() { passwordReader = origReader })

	cmd := &cobra.Command{}
	cmd.Flags().String("password", "", "password flag")
	errBuf := bytes.NewBuffer(nil)
	cmd.SetErr(errBuf)

	_, err := promptPassword(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read error")
}
