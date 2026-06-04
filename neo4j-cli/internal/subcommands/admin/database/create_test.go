// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCreate builds the `admin database create` command tree, injects a fake
// exec-fn, then executes it with args. Returns stdout, stderr, and error.
func runCreate(t *testing.T, args string, execResponses []execResponse) (string, string, error) {
	t.Helper()

	idx := 0
	orig := dbExecFn
	dbExecFn = func(_ context.Context, _ *clicfg.Config, _ *credentials.DbmsCredential, _ string, _ map[string]any) ([]map[string]any, error) {
		if idx >= len(execResponses) {
			return nil, nil
		}
		r := execResponses[idx]
		idx++
		return r.rows, r.err
	}
	t.Cleanup(func() { dbExecFn = orig })

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [{"name":"local","uri":"neo4j://localhost:7687","username":"neo4j","password":"pw","databaseName":"neo4j"}], "default-credential": "local"},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	credential := "local"
	cmd := NewCmd(cfg, &credential, dbExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

// execResponse pairs a row set with an error for sequential fake exec calls.
type execResponse struct {
	rows []map[string]any
	err  error
}

func TestCreate_HappyPath(t *testing.T) {
	_, _, err := runCreate(t, "mydb", []execResponse{
		{rows: nil, err: nil},
	})
	require.NoError(t, err)
}

func TestCreate_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runCreate(t, "mydb", []execResponse{
		{rows: nil, err: execErr},
	})
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestCreate_Wait_HappyPath(t *testing.T) {
	orig := dbWaitInterval
	dbWaitInterval = 0
	t.Cleanup(func() { dbWaitInterval = orig })

	_, stderr, err := runCreate(t, "mydb --wait", []execResponse{
		{rows: nil, err: nil},
		{rows: []map[string]any{{"currentStatus": "online"}}, err: nil},
	})
	require.NoError(t, err)
	assert.Contains(t, stderr, "Waiting for database to come online")
}

func TestCreate_Wait_Timeout(t *testing.T) {
	origTimeout := dbWaitTimeout
	origInterval := dbWaitInterval
	dbWaitTimeout = 10 * time.Millisecond
	dbWaitInterval = 0
	t.Cleanup(func() {
		dbWaitTimeout = origTimeout
		dbWaitInterval = origInterval
	})

	_, _, err := runCreate(t, "mydb --wait", []execResponse{
		{rows: nil, err: nil},
		{rows: []map[string]any{{"currentStatus": "starting"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "starting"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "starting"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "starting"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "starting"}}, err: nil},
	})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "timed out")
}

func TestCreate_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runCreate(t, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestCreate_HasWriteAnnotation(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	credential := ""
	cmd := NewCmd(cfg, &credential, dbExecFn)

	var found bool
	for _, c := range cmd.Commands() {
		if c.Name() == "create" {
			assert.Equal(t, "true", c.Annotations["write"])
			found = true
			break
		}
	}
	require.True(t, found, "create subcommand must be registered")
}
