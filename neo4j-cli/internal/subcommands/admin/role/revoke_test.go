// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

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

// withSequencedRevokeFn installs a sequenced exec-fn: first call returns
// mutationErr, subsequent calls return followUpRows/nil.
func withSequencedRevokeFn(t *testing.T, mutationErr error, followUpRows []map[string]any) {
	t.Helper()
	orig := roleExecFn
	callIdx := 0
	roleExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if callIdx == 0 {
			callIdx++
			return nil, mutationErr
		}
		callIdx++
		return followUpRows, nil
	}
	t.Cleanup(func() { roleExecFn = orig })
}

func runRevoke(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withSequencedRevokeFn(t, execErr, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"revoke"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestRevoke_HappyPath_Succeeds(t *testing.T) {
	_, _, err := runRevoke(t, "--role analyst --user alice", []map[string]any{}, nil)
	require.NoError(t, err)
}

func TestRevoke_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runRevoke(t, "--role analyst --user alice", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestRevoke_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runRevoke(t, "--role analyst --user alice", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestRevoke_MissingRole_CobraUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--user alice", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

func TestRevoke_MissingUser_CobraUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--role analyst", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}

func TestRevoke_EmitsFollowUpUserRecord(t *testing.T) {
	followUpRows := []map[string]any{
		{"user": "alice", "roles": []any{}, "passwordChangeRequired": false, "suspended": false},
	}
	withSequencedRevokeFn(t, nil, followUpRows)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"revoke", "--role", "analyst", "--user", "alice", "--format", "json"})
	require.NoError(t, cmd.Execute())

	var got []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "alice", got[0]["user"])
}
