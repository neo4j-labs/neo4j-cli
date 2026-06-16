// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRevoke builds the `admin role revoke` command tree, installs a sequenced
// exec-fn from responses, then executes with args.
func runRevoke(t *testing.T, args string, responses []struct {
	rows []map[string]any
	err  error
}) (string, string, error) {
	t.Helper()

	withSequencedExecFn(t, responses)

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
	_, _, err := runRevoke(t, "--role analyst --user alice", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},
		{rows: []map[string]any{}, err: nil},
	})
	require.NoError(t, err)
}

func TestRevoke_HappyPath_EmitsUpdatedUserRecord(t *testing.T) {
	followUpRows := []map[string]any{
		{"user": "alice", "roles": []any{}, "password_change_required": false, "suspended": false},
	}
	stdout, _, err := runRevoke(t, "--role analyst --user alice --format json", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},
		{rows: followUpRows, err: nil},
	})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "alice", got["user"])
	// Must be snake_case, not camelCase.
	_, hasPCR := got["password_change_required"]
	assert.True(t, hasPCR, "field password_change_required must be present in output")
}

func TestRevoke_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")
	_, _, err := runRevoke(t, "--role analyst --user alice", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: execErr},
	})
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestRevoke_FollowUpExecError_PropagatesError(t *testing.T) {
	followUpErr := clierr.NewValidationError("show users failed")
	_, _, err := runRevoke(t, "--role analyst --user alice", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},         // mutation succeeds
		{rows: nil, err: followUpErr}, // follow-up fails
	})
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "show users failed")
}

func TestRevoke_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("REVOKE ROLE is not supported (requires Enterprise edition)")
	_, _, err := runRevoke(t, "--role analyst --user alice", []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: enterpriseErr},
	})
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestRevoke_MissingRole_CobraUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--user alice", []struct {
		rows []map[string]any
		err  error
	}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

func TestRevoke_MissingUser_CobraUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--role analyst", []struct {
		rows []map[string]any
		err  error
	}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}
