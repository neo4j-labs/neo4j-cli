// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeUserRow_NilRoles_ReplacedWithRoleListNil(t *testing.T) {
	row := map[string]any{"user": "neo4j", "roles": nil, "suspended": false}
	got := normalizeUserRow(row)
	rl, ok := got["roles"].(roleList)
	require.True(t, ok, "expected roleList, got %T", got["roles"])
	assert.Empty(t, rl, "nil roles must normalize to empty roleList")

	jsonBytes, err := json.Marshal(rl)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(jsonBytes), "nil roleList must marshal to JSON array")
}

func TestNormalizeUserRow_SliceRoles_ReplacedWithRoleList(t *testing.T) {
	row := map[string]any{"user": "neo4j", "roles": []any{"admin", "PUBLIC"}, "suspended": false}
	got := normalizeUserRow(row)
	rl, ok := got["roles"].(roleList)
	require.True(t, ok, "expected roleList, got %T", got["roles"])
	assert.Equal(t, "admin, PUBLIC", rl.String(), "roleList.String() must return comma-joined roles")

	jsonBytes, err := json.Marshal(rl)
	require.NoError(t, err)
	assert.Equal(t, `["admin","PUBLIC"]`, string(jsonBytes), "roleList must marshal to JSON array")
}

func TestNormalizeUserRow_NilSuspended_ReplacedWithFalse(t *testing.T) {
	row := map[string]any{"user": "neo4j", "roles": nil, "suspended": nil}
	got := normalizeUserRow(row)
	assert.Equal(t, false, got["suspended"])
}

func TestNormalizeUserRow_OriginalRowUnchanged(t *testing.T) {
	original := []any{"admin", "PUBLIC"}
	row := map[string]any{"user": "neo4j", "roles": original, "suspended": nil}
	_ = normalizeUserRow(row)
	// original row must not be mutated
	assert.Equal(t, original, row["roles"])
	assert.Nil(t, row["suspended"])
}

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
