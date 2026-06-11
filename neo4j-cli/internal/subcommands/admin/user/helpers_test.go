// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptPassword_FlagSet_ReturnsFlagValue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("set-password", "", "password flag")
	require.NoError(t, cmd.Flags().Set("set-password", "secret123"))

	pw, err := promptPassword(cmd, "set-password")
	require.NoError(t, err)
	assert.Equal(t, "secret123", pw)
}

func TestPromptPassword_NoFlagNoTTY_ReturnsUsageError(t *testing.T) {
	origTTY := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return false }
	t.Cleanup(func() { dbconn.StdinIsTTY = origTTY })

	cmd := &cobra.Command{}
	cmd.Flags().String("set-password", "", "password flag")
	errBuf := bytes.NewBuffer(nil)
	cmd.SetErr(errBuf)

	_, err := promptPassword(cmd, "set-password")
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code) // usage_error
}

func TestPromptPassword_TTY_ReadsFromPasswordReader(t *testing.T) {
	origTTY := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return true }
	t.Cleanup(func() { dbconn.StdinIsTTY = origTTY })

	origReader := dbconn.PasswordReader
	dbconn.PasswordReader = func() (string, error) { return "fromtty", nil }
	t.Cleanup(func() { dbconn.PasswordReader = origReader })

	errBuf := bytes.NewBuffer(nil)
	cmd := &cobra.Command{}
	cmd.Flags().String("set-password", "", "password flag")
	cmd.SetErr(errBuf)

	pw, err := promptPassword(cmd, "set-password")
	require.NoError(t, err)
	assert.Equal(t, "fromtty", pw)
}

func TestPromptPassword_TTY_ReaderError_PropagatesError(t *testing.T) {
	origTTY := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return true }
	t.Cleanup(func() { dbconn.StdinIsTTY = origTTY })

	origReader := dbconn.PasswordReader
	dbconn.PasswordReader = func() (string, error) { return "", errors.New("read error") }
	t.Cleanup(func() { dbconn.PasswordReader = origReader })

	cmd := &cobra.Command{}
	cmd.Flags().String("set-password", "", "password flag")
	errBuf := bytes.NewBuffer(nil)
	cmd.SetErr(errBuf)

	_, err := promptPassword(cmd, "set-password")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read error")
}
