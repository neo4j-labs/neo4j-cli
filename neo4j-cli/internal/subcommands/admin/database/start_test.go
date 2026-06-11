// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runStart builds the `admin database start` command tree with a sequential
// fake exec-fn and executes with args.
func runStart(t *testing.T, args string, execResponses []execResponse) (string, string, error) {
	t.Helper()

	idx := 0
	orig := dbExecFn
	dbExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if idx >= len(execResponses) {
			return nil, nil
		}
		r := execResponses[idx]
		idx++
		return r.rows, r.err
	}
	t.Cleanup(func() { dbExecFn = orig })

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, dbExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"start"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestStart_HappyPath(t *testing.T) {
	_, _, err := runStart(t, "mydb", []execResponse{
		{rows: nil, err: nil},
		{rows: sampleDBRow, err: nil},
	})
	require.NoError(t, err)
}

func TestStart_EmitsFollowUpRecord(t *testing.T) {
	stdout, _, err := runStart(t, "mydb --format json", []execResponse{
		{rows: nil, err: nil},
		{rows: sampleDBRow, err: nil},
	})
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "mydb", got[0]["name"])
}

func TestStart_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")
	_, _, err := runStart(t, "mydb", []execResponse{
		{rows: nil, err: execErr},
	})
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestStart_Wait_HappyPath(t *testing.T) {
	orig := dbWaitInterval
	dbWaitInterval = 0
	t.Cleanup(func() { dbWaitInterval = orig })

	_, stderr, err := runStart(t, "mydb --wait", []execResponse{
		{rows: nil, err: nil},
		{rows: []map[string]any{{"currentStatus": "online"}}, err: nil},
		{rows: sampleDBRow, err: nil},
	})
	require.NoError(t, err)
	assert.Contains(t, stderr, "Waiting for database to come online")
}

func TestStart_Wait_Timeout(t *testing.T) {
	origTimeout := dbWaitTimeout
	origInterval := dbWaitInterval
	dbWaitTimeout = 10 * time.Millisecond
	dbWaitInterval = 0
	t.Cleanup(func() {
		dbWaitTimeout = origTimeout
		dbWaitInterval = origInterval
	})

	_, _, err := runStart(t, "mydb --wait", []execResponse{
		{rows: nil, err: nil},
		{rows: []map[string]any{{"currentStatus": "offline"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "offline"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "offline"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "offline"}}, err: nil},
		{rows: []map[string]any{{"currentStatus": "offline"}}, err: nil},
	})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "timed out")
}

func TestStart_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runStart(t, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestStart_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, dbExecFn)

	var found bool
	for _, c := range cmd.Commands() {
		if c.Name() == "start" {
			assert.Equal(t, "true", c.Annotations["write"])
			found = true
			break
		}
	}
	require.True(t, found, "start subcommand must be registered")
}
