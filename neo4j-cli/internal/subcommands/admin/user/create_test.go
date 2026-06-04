// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureExecFn returns an ExecFnType that records the last cypher+params and
// returns execErr. It also registers a cleanup that restores userExecFn.
func captureExecFn(t *testing.T, execErr error) (fn ExecFnType, getCypher func() string, getParams func() map[string]any) {
	t.Helper()
	orig := userExecFn
	t.Cleanup(func() { userExecFn = orig })

	var lastCypher string
	var lastParams map[string]any

	fn = func(_ context.Context, _ *clicfg.Config, _ *credentials.DbmsCredential, cypher string, params map[string]any) ([]map[string]any, error) {
		lastCypher = cypher
		lastParams = params
		return nil, execErr
	}
	return fn, func() string { return lastCypher }, func() map[string]any { return lastParams }
}

// runCreate builds the `admin user create` command tree and executes it.
// Returns stdout, stderr, captured cypher, captured params, and command error.
func runCreate(t *testing.T, args string, execErr error) (string, string, string, map[string]any, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [{"name":"local","uri":"neo4j://localhost:7687","username":"neo4j","password":"pw","databaseName":"neo4j"}], "default-credential": "local"},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	credential := "local"
	captureFn, getCypher, getParams := captureExecFn(t, execErr)
	cmd := NewCmd(cfg, &credential, captureFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), getCypher(), getParams(), execCmdErr
}

func TestCreate_HappyPath_PasswordFlag(t *testing.T) {
	_, _, cypher, params, err := runCreate(t, "alice --password secret", nil)
	require.NoError(t, err)

	assert.Contains(t, cypher, "CREATE USER $name IF NOT EXISTS")
	assert.Contains(t, cypher, "SET PASSWORD $password")
	assert.Contains(t, cypher, "CHANGE REQUIRED")
	assert.Equal(t, "alice", params["name"])
	assert.Equal(t, "secret", params["password"])
}

func TestCreate_PasswordChangeNotRequired(t *testing.T) {
	_, _, cypher, _, err := runCreate(t, "alice --password secret --password-change-required=false", nil)
	require.NoError(t, err)

	assert.Contains(t, cypher, "CHANGE NOT REQUIRED")
	assert.NotContains(t, cypher, "CHANGE REQUIRED ")
}

func TestCreate_HomeDatabase_IncludedInCypher(t *testing.T) {
	_, _, cypher, params, err := runCreate(t, "alice --password secret --home-database mydb", nil)
	require.NoError(t, err)

	assert.Contains(t, cypher, "SET HOME DATABASE $homeDatabase")
	assert.Equal(t, "mydb", params["homeDatabase"])
}

func TestCreate_NoHomeDatabase_NotIncludedInCypher(t *testing.T) {
	_, _, cypher, params, err := runCreate(t, "alice --password secret", nil)
	require.NoError(t, err)

	assert.NotContains(t, cypher, "HOME DATABASE")
	_, hasHome := params["homeDatabase"]
	assert.False(t, hasHome)
}

func TestCreate_NoPassword_NoTTY_ReturnsUsageError(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	_, _, _, _, err := runCreate(t, "alice", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "password is required")
}

func TestCreate_NoPassword_TTY_PromptsViaPasswordReader(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	origReader := passwordReader
	passwordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() { passwordReader = origReader })

	_, _, _, params, err := runCreate(t, "alice", nil)
	require.NoError(t, err)
	assert.Equal(t, "prompted-pw", params["password"])
}

func TestCreate_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("already exists")
	_, _, _, _, err := runCreate(t, "alice --password secret", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "already exists")
}

func TestCreate_WriteAnnotation(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "create" {
			assert.Equal(t, "true", sub.Annotations["write"], "create must be annotated write=true")
			return
		}
	}
	t.Fatal("create subcommand not found")
}

func TestCreate_NoArgs_CobraUsageError(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, nil))
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"create"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, strings.ToLower(execErr.Error()), "accepts 1 arg")
}
