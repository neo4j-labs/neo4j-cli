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

func runGrant(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
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
	cmd.SetArgs(append([]string{"grant"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestGrant_WildcardGraph_EmitsCorrectCypher(t *testing.T) {
	gotCypher, gotParams := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--action", "read", "--on-graph", "*", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "GRANT READ {*} ON GRAPH * ELEMENTS * TO $role", *gotCypher)
	assert.Equal(t, map[string]any{"role": "analyst"}, *gotParams)
}

func TestGrant_ScopedNodeLabelAndProperty_EmitsCorrectCypher(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--action", "read", "--on-graph", "neo4j", "--node-label", "Person", "--property", "name", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "GRANT READ {name} ON GRAPH neo4j NODES Person TO $role", *gotCypher)
}

func TestGrant_OnDatabase_EmitsCorrectCypher(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--action", "access", "--on-database", "neo4j", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "GRANT ACCESS ON DATABASE neo4j TO $role", *gotCypher)
}

func TestGrant_OnDbms_EmitsCorrectCypher(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--action", "create_role", "--on-dbms", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "GRANT CREATE ROLE ON DBMS TO $role", *gotCypher)
}

func TestGrant_DefaultGraphWhenNoResourceFlag_UsesWildcard(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--action", "read", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "GRANT READ {*} ON GRAPH * ELEMENTS * TO $role", *gotCypher)
}

func TestGrant_OnGraphAndOnDatabase_ReturnsUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--action read --on-graph '*' --on-database neo4j --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "mutually exclusive")
}

func TestGrant_NodeLabelAndRelationshipType_ReturnsUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--action read --on-graph '*' --node-label Person --relationship-type KNOWS --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "mutually exclusive")
}

func TestGrant_NodeLabelWithOnDatabase_ReturnsUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--action read --on-database neo4j --node-label Person --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "only valid with --on-graph")
}

func TestGrant_DbmsActionWithPropertyOnDbms_ReturnsUsageError(t *testing.T) {
	// --on-dbms + --property triggers the resource-qualifier exclusion check first.
	_, _, err := runGrant(t, "--action create_role --on-dbms --property name --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "only valid with --on-graph")
}

func TestGrant_DbmsActionWithPropertyOnGraph_ReturnsUsageError(t *testing.T) {
	// DBMS-level action + --property without --on-dbms triggers the DBMS-action check.
	_, _, err := runGrant(t, "--action create_role --on-graph '*' --property name --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "cannot be combined with DBMS-level action")
}

func TestGrant_UnknownAction_ReturnsUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--action nonexistent_action --role analyst", nil, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "unknown --action")
}

func TestGrant_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runGrant(t, "--action read --role analyst", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestGrant_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runGrant(t, "--action read --role analyst", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestGrant_MissingAction_CobraUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--role analyst", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action")
}

func TestGrant_MissingRole_CobraUsageError(t *testing.T) {
	_, _, err := runGrant(t, "--action read", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

func TestGrant_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	parent := NewCmd(cfg, &conn, privilegeExecFn)

	var grantCmd *cobra.Command
	for _, sub := range parent.Commands() {
		if sub.Use == "grant" {
			grantCmd = sub
			break
		}
	}
	require.NotNil(t, grantCmd)
	assert.Equal(t, "true", grantCmd.Annotations["write"])
}
