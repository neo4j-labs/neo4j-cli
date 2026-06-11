// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGet builds the `admin user get` command tree, injects a fake exec-fn
// that returns rows/execErr, then executes the command with args.
func runGet(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, rows, execErr))
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"get"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestGet_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"user": "alice", "roles": []any{"reader"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runGet(t, "alice --format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "alice", got[0]["user"])
}

func TestGet_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"user": "alice", "roles": []any{"reader"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runGet(t, "alice --format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"USER", "ROLES", "PASSWORDCHANGEREQUIRED", "SUSPENDED"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
	assert.Contains(t, stdout, "alice")
}

func TestGet_NotFound_ReturnsNotFoundError(t *testing.T) {
	stdout, _, err := runGet(t, "ghost --format json", []map[string]any{}, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code) // not_found
	assert.Contains(t, ce.Message, "ghost")
	assert.Empty(t, strings.TrimSpace(stdout))
}

func TestGet_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runGet(t, "alice --format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestGet_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runGet(t, "--format json", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestGet_RolesArray_PreservedAsArray_InJson(t *testing.T) {
	rows := []map[string]any{
		{"user": "alice", "roles": []any{"admin", "PUBLIC"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runGet(t, "alice --format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	rolesRaw, ok := got[0]["roles"]
	require.True(t, ok, "roles field must be present")
	rolesSlice, ok := rolesRaw.([]any)
	require.True(t, ok, "roles must be a JSON array ([]any), got %T: %v", rolesRaw, rolesRaw)
	require.Len(t, rolesSlice, 2)
	assert.Equal(t, "admin", rolesSlice[0])
	assert.Equal(t, "PUBLIC", rolesSlice[1])
}

func TestGet_RolesArray_RenderedAsCommaString_InTable(t *testing.T) {
	rows := []map[string]any{
		{"user": "alice", "roles": []any{"admin", "PUBLIC"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runGet(t, "alice --format table", rows, nil)
	require.NoError(t, err)

	assert.Contains(t, stdout, "admin, PUBLIC", "roles must be rendered as comma-joined string in table output")
}

func TestGet_NilRoles_RenderedAsEmptyArray_InJson(t *testing.T) {
	rows := []map[string]any{
		{"user": "community", "roles": nil, "passwordChangeRequired": false, "suspended": nil},
	}

	stdout, _, err := runGet(t, "community --format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	rolesRaw, ok := got[0]["roles"]
	require.True(t, ok)
	rolesSlice, ok := rolesRaw.([]any)
	require.True(t, ok, "expected roles to be []any, got %T", rolesRaw)
	assert.Empty(t, rolesSlice, "nil roles must normalize to empty array")
	assert.Equal(t, false, got[0]["suspended"], "nil suspended must normalize to false")
}
