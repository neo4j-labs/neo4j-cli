// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	. "github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/user"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runSetPassword builds the `admin user set-password` command with a fake
// exec-fn that records calls, then executes it with args. Returns stdout,
// stderr, and error.
func runSetPassword(t *testing.T, args string, execRows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return execRows, execErr
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewSetPasswordCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(argv)

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

var setPasswordUserRow = []map[string]any{
	{"user": "alice", "roles": []any{"reader"}, "passwordChangeRequired": false, "suspended": false},
}

func TestSetPassword_ExplicitPassword_HappyPath(t *testing.T) {
	calls := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, _ map[string]any) ([]map[string]any, error) {
		calls++
		if calls == 1 {
			// ALTER USER call
			return nil, nil
		}
		// outputUser follow-up (SHOW USERS WHERE user = ...)
		return setPasswordUserRow, nil
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewSetPasswordCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	cmd.SetArgs([]string{"alice", "--new-password", "s3cr3t", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, 2, calls)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "alice", got["user"])
}

func TestSetPassword_PasswordChangeRequired_True_SendsCorrectCypher(t *testing.T) {
	var capturedCypher string
	calls := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, _ map[string]any) ([]map[string]any, error) {
		calls++
		if calls == 1 {
			capturedCypher = cypher
			return nil, nil
		}
		return setPasswordUserRow, nil
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewSetPasswordCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))

	cmd.SetArgs([]string{"alice", "--new-password", "s3cr3t", "--password-change-required", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, capturedCypher, "SET PASSWORD CHANGE REQUIRED")
}

func TestSetPassword_PasswordChangeRequired_False_SendsNotRequired(t *testing.T) {
	var capturedCypher string
	calls := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, _ map[string]any) ([]map[string]any, error) {
		calls++
		if calls == 1 {
			capturedCypher = cypher
			return nil, nil
		}
		return setPasswordUserRow, nil
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewSetPasswordCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))

	cmd.SetArgs([]string{"alice", "--new-password", "s3cr3t", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, capturedCypher, "SET PASSWORD CHANGE NOT REQUIRED")
}

func TestSetPassword_TTYPrompt_HappyPath(t *testing.T) {
	withFakeStdinIsTTY(t, true)
	withFakePasswordReader(t, "prompted-pw", nil)

	calls := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, params map[string]any) ([]map[string]any, error) {
		calls++
		if calls == 1 {
			assert.Equal(t, "prompted-pw", params["password"])
			return nil, nil
		}
		return setPasswordUserRow, nil
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewSetPasswordCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	cmd.SetArgs([]string{"alice", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestSetPassword_NonTTY_NoPassword_ReturnsUsageError(t *testing.T) {
	withFakeStdinIsTTY(t, false)

	stdout, _, err := runSetPassword(t, "alice --format json", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "new-password")
	assert.Empty(t, stdout)
}

func TestSetPassword_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runSetPassword(t, "alice --new-password s3cr3t --format json", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestSetPassword_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewSetPasswordCmdForTest(cfg, &conn)
	assert.Equal(t, "true", cmd.Annotations["write"])
}

func TestSetPassword_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runSetPassword(t, "--new-password s3cr3t", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
