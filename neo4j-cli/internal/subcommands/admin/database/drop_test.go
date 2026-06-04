// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"bytes"
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

// runDrop builds the `admin database drop` command tree, injects a fake
// exec-fn that returns rows/execErr, then executes with args.
func runDrop(t *testing.T, args string, rows []map[string]any, execErr error, stdin string) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

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
	cmd.SetIn(strings.NewReader(stdin))

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"drop"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

// withTTY overrides the stdinIsTTY seam for the duration of t.
func withTTY(t *testing.T, isTTY bool) {
	t.Helper()
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return isTTY }
	t.Cleanup(func() { stdinIsTTY = orig })
}

func TestDrop_YesFlag_SkipsPromptAndDrops(t *testing.T) {
	_, _, err := runDrop(t, "mydb --yes", nil, nil, "")
	require.NoError(t, err)
}

func TestDrop_TTY_YesAnswer_Confirms(t *testing.T) {
	withTTY(t, true)
	_, stderr, err := runDrop(t, "mydb", nil, nil, "y\n")
	require.NoError(t, err)
	assert.Contains(t, stderr, "Drop database")
}

func TestDrop_TTY_NoAnswer_Cancels(t *testing.T) {
	withTTY(t, true)
	_, stderr, err := runDrop(t, "mydb", nil, nil, "n\n")
	require.Error(t, err)
	assert.Contains(t, stderr, "cancelled.")
}

func TestDrop_TTY_EmptyAnswer_Cancels(t *testing.T) {
	withTTY(t, true)
	_, stderr, err := runDrop(t, "mydb", nil, nil, "\n")
	require.Error(t, err)
	assert.Contains(t, stderr, "cancelled.")
}

func TestDrop_NonTTY_WithoutYes_ReturnsUsageError(t *testing.T) {
	withTTY(t, false)
	_, _, err := runDrop(t, "mydb", nil, nil, "")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "pass --yes")
}

func TestDrop_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")
	_, _, err := runDrop(t, "mydb --yes", nil, execErr, "")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestDrop_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runDrop(t, "", nil, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestDrop_HasWriteAnnotation(t *testing.T) {
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
		if c.Name() == "drop" {
			assert.Equal(t, "true", c.Annotations["write"])
			found = true
			break
		}
	}
	require.True(t, found, "drop subcommand must be registered")
}
