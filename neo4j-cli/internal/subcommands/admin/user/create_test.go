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
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// runCreate builds the `admin user create` command with a sequential fake exec-fn,
// then executes it with args. Returns stdout, stderr, and the command error.
func runCreate(t *testing.T, args string, responses []fakeResponse) (string, string, error) {
	t.Helper()

	idx := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if idx >= len(responses) {
			return nil, nil
		}
		r := responses[idx]
		idx++
		return r.rows, r.err
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newCreateCmd(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)
	flags.RegisterRwFlag(cmd)

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

var sampleUserRow = []map[string]any{
	{"user": "alice", "roles": nil, "passwordChangeRequired": true, "suspended": nil},
}

func TestUserCreate_HappyPath_ExplicitPassword(t *testing.T) {
	withFakeStdinIsTTY(t, false)

	stdout, _, err := runCreate(t, "alice --set-password s3cr3t --rw --format json", []fakeResponse{
		{rows: nil, err: nil},
		{rows: sampleUserRow, err: nil},
	})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "alice", got["user"])
}

func TestUserCreate_PasswordChangeRequired(t *testing.T) {
	tests := []struct {
		name            string
		flag            bool
		args            []string
		wantContains    string
		wantNotContains string
	}{
		{
			name:            "true emits SET PASSWORD CHANGE REQUIRED",
			flag:            true,
			args:            []string{"alice", "--set-password", "s3cr3t", "--rw"},
			wantContains:    "SET PASSWORD CHANGE REQUIRED",
			wantNotContains: "NOT REQUIRED",
		},
		{
			name:         "false emits SET PASSWORD CHANGE NOT REQUIRED",
			flag:         false,
			args:         []string{"alice", "--set-password", "s3cr3t", "--password-change-required=false", "--rw"},
			wantContains: "SET PASSWORD CHANGE NOT REQUIRED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withFakeStdinIsTTY(t, false)

			var capturedCypher string
			idx := 0
			responses := []fakeResponse{
				{rows: nil, err: nil},
				{rows: sampleUserRow, err: nil},
			}
			withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, _ map[string]any) ([]map[string]any, error) {
				if idx == 0 {
					capturedCypher = cypher
				}
				if idx >= len(responses) {
					return nil, nil
				}
				r := responses[idx]
				idx++
				return r.rows, r.err
			}))

			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			cmd := newCreateCmd(cfg, &conn)
			flags.RegisterOutputFlag(cmd, cfg)
			flags.RegisterRwFlag(cmd)
			cmd.SetOut(bytes.NewBuffer(nil))
			cmd.SetErr(bytes.NewBuffer(nil))
			cmd.SetArgs(tc.args)

			require.NoError(t, cmd.Execute())
			assert.Contains(t, capturedCypher, tc.wantContains)
			if tc.wantNotContains != "" {
				assert.NotContains(t, capturedCypher, tc.wantNotContains)
			}
		})
	}
}

func TestUserCreate_TTYPrompt_UsesPasswordReader(t *testing.T) {
	withFakeStdinIsTTY(t, true)
	withFakePasswordReader(t, "prompted-password", nil)

	var capturedParams map[string]any
	idx := 0
	responses := []fakeResponse{
		{rows: nil, err: nil},
		{rows: sampleUserRow, err: nil},
	}
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, params map[string]any) ([]map[string]any, error) {
		if idx == 0 {
			capturedParams = params
		}
		if idx >= len(responses) {
			return nil, nil
		}
		r := responses[idx]
		idx++
		return r.rows, r.err
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newCreateCmd(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)
	flags.RegisterRwFlag(cmd)

	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"alice", "--rw"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "prompted-password", capturedParams["password"])
	assert.Contains(t, errBuf.String(), "Password:")
}

func TestUserCreate_NonTTY_NoPassword_ReturnsUsageError(t *testing.T) {
	withFakeStdinIsTTY(t, false)

	_, _, err := runCreate(t, "alice --rw", nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, strings.ToLower(ce.Message), "set-password")
}

func TestUserCreate_EmitsUserRecordOnSuccess(t *testing.T) {
	withFakeStdinIsTTY(t, false)

	stdout, _, err := runCreate(t, "alice --set-password s3cr3t --rw --format json", []fakeResponse{
		{rows: nil, err: nil},
		{rows: sampleUserRow, err: nil},
	})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "alice", got["user"])
	// null roles should normalize to []
	roles, ok := got["roles"].([]any)
	require.True(t, ok, "roles should be an array in JSON output")
	assert.Empty(t, roles)
}

func TestUserCreate_ExecError_PropagatesError(t *testing.T) {
	withFakeStdinIsTTY(t, false)
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runCreate(t, "alice --set-password s3cr3t --rw", []fakeResponse{
		{rows: nil, err: execErr},
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestUserCreate_AlreadyExists_ReturnsUsageError(t *testing.T) {
	withFakeStdinIsTTY(t, false)
	alreadyExistsErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Statement.ArgumentError",
		Msg:  "Failed to create the specified user 'alice': User already exists.",
	}
	_, _, err := runCreate(t, "alice --set-password s3cr3t --rw", []fakeResponse{
		{rows: nil, err: alreadyExistsErr},
	})
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code, "already exists should have exit code 2 (usage_error)")
	assert.Contains(t, ce.Message, `"alice"`)
	assert.Contains(t, ce.Message, "already exists")
}

func TestUserCreate_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newCreateCmd(cfg, &conn)
	assert.Equal(t, "true", cmd.Annotations["write"])
}

func TestUserCreate_NoArgs_CobraUsageError(t *testing.T) {
	withFakeStdinIsTTY(t, false)

	_, _, err := runCreate(t, "--rw", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
