// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

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

// runList builds the `admin role list` command tree with a fake conn and
// the supplied exec-fn rows, then executes it with args.
func runList(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
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

func TestList_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"role": "admin", "member": "neo4j"},
		{"role": "reader", "member": "alice"},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "admin", got[0]["role"])
	assert.Equal(t, "neo4j", got[0]["member"])
	assert.Equal(t, "reader", got[1]["role"])
	assert.Equal(t, "alice", got[1]["member"])
}

func TestList_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"role": "admin", "member": "neo4j"},
	}

	stdout, _, err := runList(t, "--format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"ROLE", "MEMBER"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
	assert.Contains(t, stdout, "admin")
	assert.Contains(t, stdout, "neo4j")
}

func TestList_UserFilter_FiltersRows(t *testing.T) {
	rows := []map[string]any{
		{"role": "admin", "member": "neo4j"},
		{"role": "reader", "member": "alice"},
		{"role": "editor", "member": "alice"},
		{"role": "viewer", "member": "bob"},
	}

	stdout, _, err := runList(t, "--user alice --format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 2)
	for _, row := range got {
		assert.Equal(t, "alice", row["member"])
	}
}

func TestList_NullMember_NormalizesToEmptyString(t *testing.T) {
	rows := []map[string]any{
		{"role": "empty_role", "member": nil},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "", got[0]["member"], "nil member should normalize to empty string")
}

func TestList_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runList(t, "--format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}
