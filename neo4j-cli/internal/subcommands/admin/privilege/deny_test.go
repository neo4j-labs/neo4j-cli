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

func runDeny(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
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
	cmd.SetArgs(append([]string{"deny"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestDeny_WildcardGraph_EmitsCorrectCypher(t *testing.T) {
	gotCypher, gotParams := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"deny", "--action", "write", "--on-graph", "*", "--role", "readonly"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "DENY WRITE {*} ON GRAPH * ELEMENTS * TO $role", *gotCypher)
	assert.Equal(t, map[string]any{"role": "readonly"}, *gotParams)
}

func TestDeny_OnDatabase_EmitsCorrectCypher(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"deny", "--action", "access", "--on-database", "restricted", "--role", "readonly"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "DENY ACCESS ON DATABASE restricted TO $role", *gotCypher)
}

func TestDeny_OnGraphAndOnDbms_ReturnsUsageError(t *testing.T) {
	_, _, err := runDeny(t, "--action write --on-graph '*' --on-dbms --role readonly", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "mutually exclusive")
}

func TestDeny_PropertyWithOnDbms_ReturnsUsageError(t *testing.T) {
	_, _, err := runDeny(t, "--action write --on-dbms --property name --role readonly", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "only valid with --on-graph")
}

func TestDeny_UnknownAction_ReturnsUsageError(t *testing.T) {
	_, _, err := runDeny(t, "--action bogus_action --role readonly", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "unknown --action")
}

func TestDeny_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runDeny(t, "--action write --role readonly", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestDeny_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runDeny(t, "--action write --role readonly", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestDeny_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	parent := NewCmd(cfg, &conn, privilegeExecFn)

	var denyCmd *cobra.Command
	for _, sub := range parent.Commands() {
		if sub.Use == "deny" {
			denyCmd = sub
			break
		}
	}
	require.NotNil(t, denyCmd)
	assert.Equal(t, "true", denyCmd.Annotations["write"])
}
