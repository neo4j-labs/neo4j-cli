// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

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

func runSetPassword(t *testing.T, args string, execErr error) (string, string, string, map[string]any, error) {
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
	cmd.SetArgs(append([]string{"set-password"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), getCypher(), getParams(), execCmdErr
}

func TestSetPassword_HappyPath_PasswordFlag(t *testing.T) {
	_, _, cypher, params, err := runSetPassword(t, "alice --password newsecret", nil)
	require.NoError(t, err)

	assert.Contains(t, cypher, "ALTER USER $name SET PASSWORD $password")
	assert.Contains(t, cypher, "CHANGE NOT REQUIRED")
	assert.Equal(t, "alice", params["name"])
	assert.Equal(t, "newsecret", params["password"])
}

func TestSetPassword_PasswordChangeRequired_IncludedInCypher(t *testing.T) {
	_, _, cypher, _, err := runSetPassword(t, "alice --password newsecret --password-change-required", nil)
	require.NoError(t, err)

	assert.Contains(t, cypher, "CHANGE REQUIRED")
	assert.NotContains(t, cypher, "CHANGE NOT REQUIRED")
}

func TestSetPassword_DefaultPasswordChangeRequired_IsFalse(t *testing.T) {
	_, _, cypher, _, err := runSetPassword(t, "alice --password newsecret", nil)
	require.NoError(t, err)

	assert.Contains(t, cypher, "CHANGE NOT REQUIRED")
}

func TestSetPassword_NoPassword_NoTTY_ReturnsUsageError(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	_, _, _, _, err := runSetPassword(t, "alice", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "password is required")
}

func TestSetPassword_NoPassword_TTY_PromptsViaPasswordReader(t *testing.T) {
	origTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = origTTY })

	origReader := passwordReader
	passwordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() { passwordReader = origReader })

	_, _, _, params, err := runSetPassword(t, "alice", nil)
	require.NoError(t, err)
	assert.Equal(t, "prompted-pw", params["password"])
}

func TestSetPassword_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("user not found")
	_, _, _, _, err := runSetPassword(t, "alice --password newsecret", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "user not found")
}

func TestSetPassword_WriteAnnotation(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "set-password" {
			assert.Equal(t, "true", sub.Annotations["write"], "set-password must be annotated write=true")
			return
		}
	}
	t.Fatal("set-password subcommand not found")
}

func TestSetPassword_NoArgs_CobraUsageError(t *testing.T) {
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
	cmd.SetArgs([]string{"set-password"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, strings.ToLower(execErr.Error()), "accepts 1 arg")
}
