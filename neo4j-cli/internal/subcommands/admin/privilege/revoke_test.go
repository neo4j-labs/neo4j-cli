// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runRevoke(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

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

func TestRevoke_WildcardGraph_EmitsCorrectCypher(t *testing.T) {
	gotCypher, gotParams := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"revoke", "--action", "read", "--on-graph", "*", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "REVOKE READ {*} ON GRAPH * ELEMENTS * FROM $role", *gotCypher)
	assert.Equal(t, map[string]any{"role": "analyst"}, *gotParams)
}

func TestRevoke_RevokeTypeGrant_EmitsRevokeGrant(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"revoke", "--action", "read", "--on-graph", "*", "--role", "analyst", "--revoke-type", "grant"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "REVOKE GRANT READ {*} ON GRAPH * ELEMENTS * FROM $role", *gotCypher)
}

func TestRevoke_RevokeTypeDeny_EmitsRevokeDeny(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"revoke", "--action", "read", "--on-graph", "*", "--role", "analyst", "--revoke-type", "deny"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "REVOKE DENY READ {*} ON GRAPH * ELEMENTS * FROM $role", *gotCypher)
}

func TestRevoke_RevokeTypeCaseInsensitive_Accepted(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"revoke", "--action", "read", "--role", "analyst", "--revoke-type", "GRANT"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "REVOKE GRANT READ {*} ON GRAPH * ELEMENTS * FROM $role", *gotCypher)
}

func TestRevoke_InvalidRevokeType_ReturnsUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--action read --role analyst --revoke-type both", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "--revoke-type must be")
}

func TestRevoke_OnGraphAndOnDatabase_ReturnsUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--action read --on-graph '*' --on-database neo4j --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "mutually exclusive")
}

func TestRevoke_NodeLabelWithOnDatabase_ReturnsUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--action read --on-database neo4j --node-label Person --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "only valid with --on-graph")
}

func TestRevoke_UnknownAction_ReturnsUsageError(t *testing.T) {
	_, _, err := runRevoke(t, "--action bad_action --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "unknown --action")
}

func TestRevoke_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runRevoke(t, "--action read --role analyst", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestRevoke_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runRevoke(t, "--action read --role analyst", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestRevoke_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	parent := NewCmd(cfg, &conn, privilegeExecFn)

	var revokeCmd *cobra.Command
	for _, sub := range parent.Commands() {
		if sub.Use == "revoke" {
			revokeCmd = sub
			break
		}
	}
	require.NotNil(t, revokeCmd)
	assert.Equal(t, "true", revokeCmd.Annotations["write"])
}
