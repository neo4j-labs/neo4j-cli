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
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGet builds the `admin role get` command tree, injects a fake exec-fn
// that returns rows/execErr, then executes the command with args.
func runGet(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [{"name":"local","uri":"neo4j://localhost:7687","username":"neo4j","password":"pw","databaseName":"neo4j"}], "default-credential": "local"},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	credential := "local"
	cmd := NewCmd(cfg, &credential, roleExecFn)
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
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "admin"},
	}

	stdout, _, err := runGet(t, "admin --format json", rows, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "admin", got[0]["role"])
}

func TestGet_HappyPath_FormatTable(t *testing.T) {
	rows := []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "admin"},
	}

	stdout, _, err := runGet(t, "admin --format table", rows, nil)
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	assert.Contains(t, upper, "ROLE")
	assert.Contains(t, stdout, "admin")
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

	_, _, err := runGet(t, "admin --format json", nil, execErr)
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
