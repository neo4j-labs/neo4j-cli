// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

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
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override userExecFn with a fake.
func testConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}

// fakeExecFn returns an adminutil.ExecFn that returns the supplied rows or error.
// It also registers a cleanup that restores userExecFn to its original value.
func fakeExecFn(t *testing.T, rows []map[string]any, execErr error) adminutil.ExecFn {
	t.Helper()
	orig := userExecFn
	t.Cleanup(func() { userExecFn = orig })
	return func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return rows, execErr
	}
}

// runList builds the `admin user list` command tree with a fake conn and
// the supplied exec-fn rows, then executes it with args.
func runList(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
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
	cmd.SetArgs(append([]string{"list"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestList_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"user": "neo4j", "roles": []any{"admin"}, "passwordChangeRequired": false, "suspended": false},
		{"user": "alice", "roles": []any{"reader"}, "passwordChangeRequired": true, "suspended": false},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "neo4j", got[0]["user"])
	assert.Equal(t, "alice", got[1]["user"])
}

func TestList_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"user": "neo4j", "roles": []any{"admin"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runList(t, "--format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"USER", "ROLES", "PASSWORDCHANGEREQUIRED", "SUSPENDED"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
	assert.Contains(t, stdout, "neo4j")
}

func TestList_EmptyResult_FormatJson_RendersEmptyArray(t *testing.T) {
	stdout, _, err := runList(t, "--format json", []map[string]any{}, nil)
	require.NoError(t, err)

	trimmed := strings.TrimSpace(stdout)
	assert.Equal(t, "[]", trimmed)
}

func TestList_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runList(t, "--format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}
