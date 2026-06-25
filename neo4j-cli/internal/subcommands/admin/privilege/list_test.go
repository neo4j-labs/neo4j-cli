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

// capture records the cypher and params passed to the exec seam.
type capture struct {
	cypher string
	params map[string]any
	called bool
}

// withCapturingExecFn replaces privilegeExecFn with a fake that records its
// cypher/params into cap and returns the supplied rows or error.
func withCapturingExecFn(t *testing.T, cap *capture, rows []map[string]any, execErr error) {
	t.Helper()
	orig := privilegeExecFn
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) {
		cap.called = true
		cap.cypher = cypher
		cap.params = params
		return rows, execErr
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}

// runList builds the `admin privilege list` command tree with a fake conn and
// the supplied capturing exec-fn, then executes it with args.
func runList(t *testing.T, args string, cap *capture, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withCapturingExecFn(t, cap, rows, execErr)

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

func TestList_NoFilter_ShowPrivileges(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "read", "resource": "all_properties", "segment": "NODE(*)", "role": "admin"},
	}
	cap := &capture{}

	stdout, _, err := runList(t, "--format json", cap, rows, nil)
	require.NoError(t, err)

	assert.Equal(t, "SHOW PRIVILEGES", cap.cypher)
	assert.Nil(t, cap.params)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	for _, col := range privilegeFields {
		_, ok := got[0][col]
		assert.True(t, ok, "missing column %q", col)
	}
	assert.Equal(t, "GRANTED", got[0]["access"])
	assert.Equal(t, "admin", got[0]["role"])
}

func TestList_FormatTable_RendersColumns(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "read", "resource": "all_properties", "segment": "NODE(*)", "role": "admin"},
	}
	cap := &capture{}

	stdout, _, err := runList(t, "--format table", cap, rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"ACCESS", "ACTION", "RESOURCE", "SEGMENT", "ROLE"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
}

func TestList_RoleFilter_ShowRolePrivileges(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "match", "resource": "graph", "segment": "NODE(*)", "role": "analyst"},
	}
	cap := &capture{}

	_, _, err := runList(t, "--role analyst --format json", cap, rows, nil)
	require.NoError(t, err)

	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", cap.cypher)
	assert.Equal(t, "analyst", cap.params["name"])
}

func TestList_UserFilter_ShowUserPrivileges(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "match", "resource": "graph", "segment": "NODE(*)", "role": "reader"},
	}
	cap := &capture{}

	_, _, err := runList(t, "--user alice --format json", cap, rows, nil)
	require.NoError(t, err)

	assert.Equal(t, "SHOW USER $name PRIVILEGES", cap.cypher)
	assert.Equal(t, "alice", cap.params["name"])
}

func TestList_BothFilters_UsageError(t *testing.T) {
	cap := &capture{}

	_, _, err := runList(t, "--role analyst --user alice --format json", cap, nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.False(t, cap.called, "seam must not be called when both filters are set")
}

// runListSequenced builds the `admin privilege list` command tree with a fake
// conn and a recording sequenced exec-fn, then executes it with args. It returns
// the recorded calls so tests can assert the existence-check round-trip.
func runListSequenced(t *testing.T, args string, responses []struct {
	rows []map[string]any
	err  error
}) (*[]sequencedCall, error) {
	t.Helper()

	calls := &[]sequencedCall{}
	withRecordingSequencedExecFn(t, calls, responses)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"list"}, argv...))

	return calls, cmd.Execute()
}

func TestList_RoleFilter_Nonexistent_ValidationError(t *testing.T) {
	calls, err := runListSequenced(t, "--role missing --format json", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: []map[string]any{}, err: nil},
		{rows: []map[string]any{}, err: nil},
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, `role "missing" does not exist`)

	require.Len(t, *calls, 2)
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", (*calls)[0].cypher)
	assert.Equal(t, "SHOW ROLES YIELD role WHERE role = $name RETURN role", (*calls)[1].cypher)
	assert.Equal(t, "missing", (*calls)[1].params["name"])
}

func TestList_UserFilter_Nonexistent_ValidationError(t *testing.T) {
	calls, err := runListSequenced(t, "--user ghost --format json", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: []map[string]any{}, err: nil},
		{rows: []map[string]any{}, err: nil},
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 6, ce.Code)
	assert.Contains(t, ce.Message, `user "ghost" does not exist`)

	require.Len(t, *calls, 2)
	assert.Equal(t, "SHOW USER $name PRIVILEGES", (*calls)[0].cypher)
	assert.Equal(t, "SHOW USERS YIELD user WHERE user = $name RETURN user", (*calls)[1].cypher)
	assert.Equal(t, "ghost", (*calls)[1].params["name"])
}

func TestList_RoleFilter_ExistsButNoPrivileges_EmptyOK(t *testing.T) {
	calls, err := runListSequenced(t, "--role analyst --format json", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: []map[string]any{}, err: nil},
		{rows: []map[string]any{{"role": "analyst"}}, err: nil},
	})
	require.NoError(t, err)

	require.Len(t, *calls, 2)
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", (*calls)[0].cypher)
	assert.Equal(t, "SHOW ROLES YIELD role WHERE role = $name RETURN role", (*calls)[1].cypher)
}

func TestList_RoleFilter_NonEmpty_NoExistenceCheck(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "match", "resource": "graph", "segment": "NODE(*)", "role": "analyst"},
	}
	calls, err := runListSequenced(t, "--role analyst --format json", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: rows, err: nil},
	})
	require.NoError(t, err)

	require.Len(t, *calls, 1, "a target with privileges must not trigger an existence check")
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", (*calls)[0].cypher)
}

func TestList_NoFilter_Empty_NoExistenceCheck(t *testing.T) {
	calls, err := runListSequenced(t, "--format json", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: []map[string]any{}, err: nil},
	})
	require.NoError(t, err)

	require.Len(t, *calls, 1, "unfiltered SHOW PRIVILEGES must not trigger an existence check")
	assert.Equal(t, "SHOW PRIVILEGES", (*calls)[0].cypher)
}

func TestList_EnterpriseOnlyError_PropagatesValidationError(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("SHOW PRIVILEGES is not supported (requires Enterprise edition)")
	cap := &capture{}

	_, _, err := runList(t, "--format json", cap, nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "requires Enterprise edition")
}
