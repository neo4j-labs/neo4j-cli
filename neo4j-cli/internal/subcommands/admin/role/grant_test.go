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

// runGrant builds the `admin role grant` command tree, installs a sequenced
// exec-fn (mutation call 1 with execErr, follow-up SHOW USERS call 2 with empty
// rows), then executes with args.
func runGrant(t *testing.T, args string, execErr error) (string, string, error) {
	t.Helper()

	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: execErr},
		{rows: []map[string]any{}, err: nil},
	}
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
	cmd.SetArgs(append([]string{"grant"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestGrant_HappyPath_Succeeds(t *testing.T) {
	_, _, err := runGrant(t, "--role analyst --user alice", nil)
	require.NoError(t, err)
}

func TestGrant_HappyPath_EmitsUpdatedUserRecord(t *testing.T) {
	followUpRows := []map[string]any{
		{"user": "alice", "roles": []any{"analyst"}, "password_change_required": false, "suspended": false},
	}

	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},
		{rows: followUpRows, err: nil},
	}
	withSequencedExecFn(t, responses)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--role", "analyst", "--user", "alice", "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "alice", got["user"])
	// Must be snake_case, not camelCase.
	_, hasPCR := got["password_change_required"]
	assert.True(t, hasPCR, "field password_change_required must be present in output")
}

func TestGrant_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")
	_, _, err := runGrant(t, "--role analyst --user alice", execErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestGrant_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("GRANT ROLE is not supported (requires Enterprise edition)")
	_, _, err := runGrant(t, "--role analyst --user alice", enterpriseErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestGrant_MissingRole_CobraUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--user alice", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

func TestGrant_MissingUser_CobraUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--role analyst", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}
