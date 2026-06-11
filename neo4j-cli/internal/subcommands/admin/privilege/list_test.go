// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

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
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override privilegeExecFn with a fake.
func testConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}

// withFakeExecFn replaces privilegeExecFn for the duration of t with a fake
// that returns the supplied rows or error. For list tests only — mutation
// tests should use withSequencedMutationFn instead to handle the follow-up
// emitRolePrivileges call.
func withFakeExecFn(t *testing.T, rows []map[string]any, execErr error) {
	t.Helper()
	orig := privilegeExecFn
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return rows, execErr
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}

// captureExecFn replaces privilegeExecFn for the duration of t with a
// sequenced fake: the FIRST call (the mutation) records its cypher and params
// and returns empty rows/nil. Subsequent calls (the emitRolePrivileges
// follow-up) return the supplied followUpRows.
func captureExecFn(t *testing.T, followUpRows []map[string]any) (gotCypher *string, gotParams *map[string]any) {
	t.Helper()
	orig := privilegeExecFn
	var cypher string
	var params map[string]any
	callIdx := 0
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, q string, p map[string]any) ([]map[string]any, error) {
		if callIdx == 0 {
			cypher = q
			params = p
			callIdx++
			return nil, nil
		}
		callIdx++
		return followUpRows, nil
	}
	t.Cleanup(func() { privilegeExecFn = orig })
	return &cypher, &params
}

// withSequencedMutationFn installs a sequenced exec-fn: first call returns
// mutationErr, subsequent calls return followUpRows/nil. Used by runMutation
// and runRevoke to handle the emitRolePrivileges follow-up call.
func withSequencedMutationFn(t *testing.T, mutationRows []map[string]any, mutationErr error, followUpRows []map[string]any) {
	t.Helper()
	orig := privilegeExecFn
	callIdx := 0
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if callIdx == 0 {
			callIdx++
			return mutationRows, mutationErr
		}
		callIdx++
		return followUpRows, nil
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}

// runList builds the `admin privilege list` command tree with a fake conn and
// the supplied exec-fn rows, then executes it with args.
func runList(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"list"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestList_DefaultNoFilter_ExecutesShowPrivileges(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"list", "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "SHOW PRIVILEGES", *gotCypher)
}

func TestList_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "reader"},
		{"access": "GRANTED", "action": "find", "resource": "all_properties", "graph": "*", "segment": "NODE(*)", "role": "reader"},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "GRANTED", got[0]["access"])
}

func TestList_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "reader"},
	}

	stdout, _, err := runList(t, "--format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	assert.Contains(t, upper, "ACCESS")
	assert.Contains(t, stdout, "GRANTED")
}

func TestList_RoleFilter_ExecutesShowRolePrivileges(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "analyst"},
	})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"list", "--role", "analyst", "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", *gotCypher)
}

func TestList_UserFilter_ExecutesShowUserPrivileges(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "reader"},
	})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"list", "--user", "alice", "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "SHOW USER $name PRIVILEGES", *gotCypher)
}

func TestList_RoleAndUserBothSet_ReturnsUsageError(t *testing.T) {
	withFakeExecFn(t, nil, nil)

	_, _, err := runList(t, "--role analyst --user alice --format json", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "mutually exclusive")
}

func TestList_EmptyResult_FormatJson_RendersEmptyArray(t *testing.T) {
	stdout, _, err := runList(t, "--format json", []map[string]any{}, nil)
	require.NoError(t, err)

	trimmed := strings.TrimSpace(stdout)
	assert.Equal(t, "[]", trimmed)
}

func TestList_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runList(t, "--format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestList_UnsupportedAdministrationCommand_ReturnsEnterpriseHint(t *testing.T) {
	execErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runList(t, "--format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}
