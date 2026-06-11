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

// withSequencedGrantFn installs a sequenced exec-fn: first call returns
// mutationErr, subsequent calls return followUpRows/nil.
func withSequencedGrantFn(t *testing.T, mutationErr error, followUpRows []map[string]any) {
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

func runGrant(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withSequencedGrantFn(t, execErr, []map[string]any{})

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
	cmd.SetArgs(append([]string{"grant"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestGrant_HappyPath_Succeeds(t *testing.T) {
	_, _, err := runGrant(t, "--role analyst --user alice", []map[string]any{}, nil)
	require.NoError(t, err)
}

func TestGrant_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runGrant(t, "--role analyst --user alice", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestGrant_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runGrant(t, "--role analyst --user alice", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestGrant_MissingRole_CobraUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--user alice", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

func TestGrant_MissingUser_CobraUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--role analyst", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}

func TestGrant_EmitsFollowUpUserRecord(t *testing.T) {
	followUpRows := []map[string]any{
		{"user": "alice", "roles": []any{"analyst"}, "passwordChangeRequired": false, "suspended": false},
	}
	withSequencedGrantFn(t, nil, followUpRows)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"grant", "--role", "analyst", "--user", "alice", "--format", "json"})
	require.NoError(t, cmd.Execute())

	var got []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "alice", got[0]["user"])
}
