// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

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

// runList builds the `admin database list` command tree with a fake conn and
// the supplied exec-fn rows, then executes it with args.
func runList(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

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
	cmd.SetArgs(append([]string{"list"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestList_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"name": "neo4j", "type": "standard", "currentStatus": "online", "access": "read-write", "default": true},
		{"name": "system", "type": "system", "currentStatus": "online", "access": "read-write", "default": false},
	}

	stdout, _, err := runList(t, "--format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "neo4j", got[0]["name"])
	assert.Equal(t, "system", got[1]["name"])
}

func TestList_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"name": "neo4j", "type": "standard", "currentStatus": "online", "access": "read-write", "default": true},
	}

	stdout, _, err := runList(t, "--format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"NAME", "CURRENT_STATUS", "TYPE", "DEFAULT"} {
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
