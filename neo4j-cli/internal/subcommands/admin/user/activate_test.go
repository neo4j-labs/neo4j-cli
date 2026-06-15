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
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// runActivate builds the `admin user activate` command with a sequential fake
// exec-fn and executes it with args. The first call is the ALTER USER statement;
// the second call is the SHOW USERS follow-up from outputUser.
func runActivate(t *testing.T, args string, execResponses []fakeResponse) (string, string, error) {
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
	cmd := newActivateCmd(cfg, &conn)
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

func TestUserActivate_HappyPath_EmitsUpdatedRecord(t *testing.T) {
	responses := []fakeResponse{
		// ALTER USER ... SET STATUS ACTIVE
		{rows: nil, err: nil},
		// SHOW USERS follow-up from outputUser
		{rows: sampleActiveUserRow, err: nil},
	}

	stdout, _, err := runActivate(t, "alice --format json", responses)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "alice", got["user"])
	assert.Equal(t, false, got["suspended"])
}

func TestUserActivate_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("community edition does not support user activation")

	responses := []fakeResponse{
		{rows: nil, err: execErr},
	}

	_, _, err := runActivate(t, "alice --format json", responses)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "community edition does not support user activation")
}

func TestUserActivate_NotFound_ReturnsNotFoundError(t *testing.T) {
	notFoundErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Statement.ArgumentError",
		Msg:  "User 'ghost' does not exist.",
	}
	responses := []fakeResponse{
		{rows: nil, err: notFoundErr},
	}
	_, _, err := runActivate(t, "ghost --format json", responses)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Contains(t, ce.Message, `"ghost"`)
}

func TestUserActivate_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runActivate(t, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestUserActivate_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newActivateCmd(cfg, &conn)
	assert.Equal(t, "true", cmd.Annotations["write"])
}
