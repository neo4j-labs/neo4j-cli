// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	. "github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/user"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runList builds the `admin user list` command with a fake exec-fn that
// returns rows/execErr, then executes it with args.
func runList(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return rows, execErr
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewListCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(argv)

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestUserList_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"user": "neo4j", "roles": []any{"admin"}, "passwordChangeRequired": false, "suspended": false},
		{"user": "alice", "roles": []any{"reader", "editor"}, "passwordChangeRequired": true, "suspended": false},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "neo4j", got[0]["user"])
	assert.Equal(t, "alice", got[1]["user"])
	// JSON format: roles must be an array, not a string
	roles, ok := got[0]["roles"].([]any)
	require.True(t, ok, "roles should be an array in JSON output")
	assert.Equal(t, []any{"admin"}, roles)
}

func TestUserList_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"user": "neo4j", "roles": []any{"admin"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runList(t, "--format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"USER", "ROLES", "PASSWORD_CHANGE_REQUIRED", "SUSPENDED"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
	assert.Contains(t, stdout, "neo4j")
}

func TestUserList_RolesJoined_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"user": "alice", "roles": []any{"reader", "editor"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runList(t, "--format table", rows, nil)
	require.NoError(t, err)

	// table format: multiple roles should be comma-joined, not printed as a Go slice literal
	assert.Contains(t, stdout, "reader,editor")
	assert.NotContains(t, stdout, "[reader")
}

func TestUserList_NullRoles_CommunityEdition_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"user": "neo4j", "roles": nil, "passwordChangeRequired": false, "suspended": nil},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	// null roles should normalize to empty array
	roles, ok := got[0]["roles"].([]any)
	require.True(t, ok, "roles should be an array after normalization")
	assert.Empty(t, roles)
	// null suspended should normalize to false
	assert.Equal(t, false, got[0]["suspended"])
}

func TestUserList_EmptyResult_FormatJson_RendersEmptyArray(t *testing.T) {
	stdout, _, err := runList(t, "--format json", []map[string]any{}, nil)
	require.NoError(t, err)

	trimmed := strings.TrimSpace(stdout)
	assert.Equal(t, "[]", trimmed)
}

func TestUserList_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runList(t, "--format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}
