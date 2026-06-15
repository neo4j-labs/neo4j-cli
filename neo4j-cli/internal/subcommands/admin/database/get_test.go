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

// runGet builds the `admin database get` command tree, injects a fake exec-fn
// that returns rows/execErr, then executes the command with args.
func runGet(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
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
	cmd.SetArgs(append([]string{"get"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestGet_HappyPath_FormatJson(t *testing.T) {
	rows := []map[string]any{
		{"name": "neo4j", "type": "standard", "currentStatus": "online", "access": "read-write", "default": true},
	}

	stdout, _, err := runGet(t, "neo4j --format json", rows, nil)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "neo4j", got["name"])
}

func TestGet_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"name": "neo4j", "type": "standard", "currentStatus": "online", "access": "read-write", "default": true},
	}

	stdout, _, err := runGet(t, "neo4j --format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"NAME", "TYPE", "ACCESS", "CURRENT_STATUS", "REQUESTED_STATUS", "STATUS_MESSAGE", "ADDRESS", "ROLE", "WRITER", "DEFAULT", "HOME", "DATABASE_ID"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
	assert.Contains(t, stdout, "neo4j")
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

	_, _, err := runGet(t, "neo4j --format json", nil, execErr)
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
