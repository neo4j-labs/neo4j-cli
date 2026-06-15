// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

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
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runSuspend builds the `admin user suspend` command with a sequential fake
// exec-fn and executes it with args. The first call is the ALTER USER statement;
// the second call is the SHOW USERS follow-up from outputUser.
func runSuspend(t *testing.T, args string, execResponses []fakeResponse) (string, string, error) {
	t.Helper()

	idx := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if idx >= len(execResponses) {
			return nil, nil
		}
		r := execResponses[idx]
		idx++
		return r.rows, r.err
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newSuspendCmd(cfg, &conn)
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

// sampleSuspendedUserRow is a realistic SHOW USERS row for a suspended user.
var sampleSuspendedUserRow = []map[string]any{
	{"user": "alice", "roles": []any{"reader"}, "passwordChangeRequired": false, "suspended": true},
}

// sampleActiveUserRow is a realistic SHOW USERS row for an active user.
var sampleActiveUserRow = []map[string]any{
	{"user": "alice", "roles": []any{"reader"}, "passwordChangeRequired": false, "suspended": false},
}

func TestUserSuspend_HappyPath_EmitsUpdatedRecord(t *testing.T) {
	responses := []fakeResponse{
		// ALTER USER ... SET STATUS SUSPENDED
		{rows: nil, err: nil},
		// SHOW USERS follow-up from outputUser
		{rows: sampleSuspendedUserRow, err: nil},
	}

	stdout, _, err := runSuspend(t, "alice --format json", responses)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "alice", got["user"])
	assert.Equal(t, true, got["suspended"])
}

func TestUserSuspend_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("community edition does not support user suspension")

	responses := []fakeResponse{
		{rows: nil, err: execErr},
	}

	_, _, err := runSuspend(t, "alice --format json", responses)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "community edition does not support user suspension")
}

func TestUserSuspend_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runSuspend(t, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestUserSuspend_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newSuspendCmd(cfg, &conn)
	assert.Equal(t, "true", cmd.Annotations["write"])
}
