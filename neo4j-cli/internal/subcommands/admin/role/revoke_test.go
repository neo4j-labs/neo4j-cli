// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runRevoke(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)

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
