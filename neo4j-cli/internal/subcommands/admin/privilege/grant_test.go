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

// runMutation builds the `admin privilege` parent command and executes the
// subcommand named by sub (e.g. "grant" or "deny") with the given args.
func runMutation(t *testing.T, sub, args string, rows []map[string]any, execErr error) (string, string, error) {
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
	cmd.SetArgs(append([]string{sub}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

// TestGrantDeny_WildcardGraph_EmitsCorrectCypher verifies that both grant and
// deny produce the expected GRANT/DENY prefix with a wildcard graph resource.
func TestGrantDeny_WildcardGraph_EmitsCorrectCypher(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		action         string
		role           string
		expectedCypher string
		expectedParams map[string]any
	}{
		{
			sub:            "grant",
			action:         "read",
			role:           "analyst",
			expectedCypher: "GRANT READ {*} ON GRAPH * ELEMENTS * TO $role",
			expectedParams: map[string]any{"role": "analyst"},
		},
		{
			sub:            "deny",
			action:         "write",
			role:           "readonly",
			expectedCypher: "DENY WRITE {*} ON GRAPH * ELEMENTS * TO $role",
			expectedParams: map[string]any{"role": "readonly"},
		},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			gotCypher, gotParams := captureExecFn(t, []map[string]any{})

			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			cmd := NewCmd(cfg, &conn, privilegeExecFn)

			out := bytes.NewBuffer(nil)
			errBuf := bytes.NewBuffer(nil)
			cmd.SetOut(out)
			cmd.SetErr(errBuf)
			cmd.SetArgs([]string{tc.sub, "--action", tc.action, "--on-graph", "*", "--role", tc.role})

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCypher, *gotCypher)
			assert.Equal(t, tc.expectedParams, *gotParams)
		})
	}
}

func TestGrantDeny_ScopedNodeLabelAndProperty_EmitsCorrectCypher(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		expectedCypher string
	}{
		{"grant", "GRANT READ {name} ON GRAPH neo4j NODES Person TO $role"},
		{"deny", "DENY READ {name} ON GRAPH neo4j NODES Person TO $role"},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			gotCypher, _ := captureExecFn(t, []map[string]any{})

			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			cmd := NewCmd(cfg, &conn, privilegeExecFn)

			out := bytes.NewBuffer(nil)
			errBuf := bytes.NewBuffer(nil)
			cmd.SetOut(out)
			cmd.SetErr(errBuf)
			cmd.SetArgs([]string{tc.sub, "--action", "read", "--on-graph", "neo4j", "--node-label", "Person", "--property", "name", "--role", "analyst"})

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCypher, *gotCypher)
		})
	}
}

func TestGrantDeny_OnDatabase_EmitsCorrectCypher(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		action         string
		database       string
		expectedCypher string
	}{
		{"grant", "access", "neo4j", "GRANT ACCESS ON DATABASE neo4j TO $role"},
		{"deny", "access", "restricted", "DENY ACCESS ON DATABASE restricted TO $role"},
	} {
		t.Run(tc.sub, func(t *testing.T) {
			gotCypher, _ := captureExecFn(t, []map[string]any{})

			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			cmd := NewCmd(cfg, &conn, privilegeExecFn)

			out := bytes.NewBuffer(nil)
			errBuf := bytes.NewBuffer(nil)
			cmd.SetOut(out)
			cmd.SetErr(errBuf)
			cmd.SetArgs([]string{tc.sub, "--action", tc.action, "--on-database", tc.database, "--role", "analyst"})

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCypher, *gotCypher)
		})
	}
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

func TestGrantDeny_OnGraphAndOnDatabase_ReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action read --on-graph '*' --on-database neo4j --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "mutually exclusive")
		})
	}
}

func TestGrantDeny_NodeLabelAndRelationshipType_ReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action read --on-graph '*' --node-label Person --relationship-type KNOWS --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "mutually exclusive")
		})
	}
}

func TestGrantDeny_NodeLabelWithOnDatabase_ReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action read --on-database neo4j --node-label Person --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "only valid with --on-graph")
		})
	}
}

func TestGrantDeny_DbmsActionWithPropertyOnDbms_ReturnsUsageError(t *testing.T) {
	// --on-dbms + --property triggers the resource-qualifier exclusion check first.
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action create_role --on-dbms --property name --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "only valid with --on-graph")
		})
	}
}

func TestGrantDeny_DbmsActionWithPropertyOnGraph_ReturnsUsageError(t *testing.T) {
	// DBMS-level action + --property without --on-dbms triggers the DBMS-action check.
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action create_role --on-graph '*' --property name --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "cannot be combined with DBMS-level action")
		})
	}
}

func TestGrantDeny_UnknownAction_ReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action nonexistent_action --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "unknown --action")
		})
	}
}

func TestGrantDeny_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action read --role analyst", nil, enterpriseErr)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "Enterprise edition")
		})
	}
}

func TestGrantDeny_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action read --role analyst", nil, execErr)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "bolt connection refused")
		})
	}
}

func TestGrantDeny_MissingAction_CobraUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--role analyst", nil, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "action")
		})
	}
}

func TestGrantDeny_MissingRole_CobraUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action read", nil, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "role")
		})
	}
}

func TestGrantDeny_HasWriteAnnotation(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			parent := NewCmd(cfg, &conn, privilegeExecFn)

			var found *cobra.Command
			for _, c := range parent.Commands() {
				if c.Use == sub {
					found = c
					break
				}
			}
			require.NotNil(t, found)
			assert.Equal(t, "true", found.Annotations["write"])
		})
	}
}
