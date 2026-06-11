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

// TestGrantDeny_PropertyBearer_EmitsPropertyQualifier verifies that
// catPropertyBearer actions (READ, MATCH, SET PROPERTY, MERGE) include {*}
// on wildcard graph.
func TestGrantDeny_PropertyBearer_EmitsPropertyQualifier(t *testing.T) {
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
			action:         "match",
			role:           "readonly",
			expectedCypher: "DENY MATCH {*} ON GRAPH * ELEMENTS * TO $role",
			expectedParams: map[string]any{"role": "readonly"},
		},
	} {
		t.Run(tc.sub+"/"+tc.action, func(t *testing.T) {
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

// TestGrantDeny_GraphOnly_NoPropertyQualifier verifies that catGraphOnly
// actions (TRAVERSE, WRITE, CREATE, DELETE, LOAD, ALL GRAPH PRIVILEGES) emit
// ON GRAPH without a {*} property qualifier.
func TestGrantDeny_GraphOnly_NoPropertyQualifier(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		action         string
		expectedCypher string
	}{
		{"grant", "traverse", "GRANT TRAVERSE ON GRAPH neo4j ELEMENTS * TO $role"},
		{"deny", "write", "DENY WRITE ON GRAPH * ELEMENTS * TO $role"},
		{"grant", "create", "GRANT CREATE ON GRAPH * ELEMENTS * TO $role"},
		{"deny", "delete", "DENY DELETE ON GRAPH * ELEMENTS * TO $role"},
		{"grant", "load", "GRANT LOAD ON GRAPH * ELEMENTS * TO $role"},
		{"deny", "all_graph_privileges", "DENY ALL GRAPH PRIVILEGES ON GRAPH * ELEMENTS * TO $role"},
	} {
		t.Run(tc.sub+"/"+tc.action, func(t *testing.T) {
			gotCypher, _ := captureExecFn(t, []map[string]any{})

			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			cmd := NewCmd(cfg, &conn, privilegeExecFn)

			out := bytes.NewBuffer(nil)
			errBuf := bytes.NewBuffer(nil)
			cmd.SetOut(out)
			cmd.SetErr(errBuf)

			graphArg := "*"
			if tc.action == "traverse" {
				graphArg = "neo4j"
			}
			cmd.SetArgs([]string{tc.sub, "--action", tc.action, "--on-graph", graphArg, "--role", "analyst"})

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCypher, *gotCypher)
		})
	}
}

// TestGrantDeny_GraphOnly_PropertyFlagReturnsUsageError verifies that
// catGraphOnly actions reject --property.
func TestGrantDeny_GraphOnly_PropertyFlagReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action traverse --on-graph '*' --property name --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "--property is not valid")
		})
	}
}

// TestGrantDeny_SetLabel_EmitsCorrectCypher verifies that SET LABEL produces
// the label-form Cypher.
func TestGrantDeny_SetLabel_EmitsCorrectCypher(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		expectedCypher string
	}{
		{"grant", "GRANT SET LABEL Person ON GRAPH neo4j TO $role"},
		{"deny", "DENY SET LABEL Person ON GRAPH neo4j TO $role"},
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
			cmd.SetArgs([]string{tc.sub, "--action", "set_label", "--node-label", "Person", "--on-graph", "neo4j", "--role", "analyst"})

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCypher, *gotCypher)
		})
	}
}

// TestGrantDeny_SetLabel_MissingNodeLabel_ReturnsUsageError verifies that
// SET LABEL without --node-label returns a usage error.
func TestGrantDeny_SetLabel_MissingNodeLabel_ReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action set_label --on-graph neo4j --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "--node-label is required")
		})
	}
}

// TestGrantDeny_ScopedNodeLabelAndProperty_EmitsCorrectCypher verifies the
// property+entity clause for catPropertyBearer actions.
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

// TestGrantDeny_OnDatabase_EmitsCorrectCypher verifies that database-level
// actions use ON DATABASE without entity/property qualifiers.
func TestGrantDeny_OnDatabase_EmitsCorrectCypher(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		action         string
		database       string
		expectedCypher string
	}{
		{"grant", "access", "neo4j", "GRANT ACCESS ON DATABASE neo4j TO $role"},
		{"deny", "access", "restricted", "DENY ACCESS ON DATABASE restricted TO $role"},
		{"grant", "start", "*", "GRANT START ON DATABASE * TO $role"},
		{"deny", "all_database_privileges", "mydb", "DENY ALL DATABASE PRIVILEGES ON DATABASE mydb TO $role"},
	} {
		t.Run(tc.sub+"/"+tc.action, func(t *testing.T) {
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

// TestGrantDeny_Database_DefaultsToWildcard verifies that a database-level
// action without --on-database defaults to ON DATABASE *.
func TestGrantDeny_Database_DefaultsToWildcard(t *testing.T) {
	gotCypher, _ := captureExecFn(t, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"grant", "--action", "access", "--role", "analyst"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, "GRANT ACCESS ON DATABASE * TO $role", *gotCypher)
}

// TestGrantDeny_Database_OnGraphFlagReturnsUsageError verifies that
// catDatabase actions reject --on-graph.
func TestGrantDeny_Database_OnGraphFlagReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action access --on-graph '*' --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "requires --on-database")
		})
	}
}

// TestGrant_OnDbms_EmitsCorrectCypher verifies that DBMS-level actions emit
// ON DBMS when --on-dbms is set.
func TestGrant_OnDbms_EmitsCorrectCypher(t *testing.T) {
	for _, tc := range []struct {
		sub            string
		action         string
		expectedCypher string
	}{
		{"grant", "create_role", "GRANT CREATE ROLE ON DBMS TO $role"},
		{"deny", "all_dbms_privileges", "DENY ALL DBMS PRIVILEGES ON DBMS TO $role"},
	} {
		t.Run(tc.sub+"/"+tc.action, func(t *testing.T) {
			gotCypher, _ := captureExecFn(t, []map[string]any{})

			cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
			conn := testConn()
			cmd := NewCmd(cfg, &conn, privilegeExecFn)

			out := bytes.NewBuffer(nil)
			errBuf := bytes.NewBuffer(nil)
			cmd.SetOut(out)
			cmd.SetErr(errBuf)
			cmd.SetArgs([]string{tc.sub, "--action", tc.action, "--on-dbms", "--role", "analyst"})

			err := cmd.Execute()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedCypher, *gotCypher)
		})
	}
}

// TestGrantDeny_Dbms_MissingOnDbms_ReturnsUsageError verifies that DBMS-level
// actions require --on-dbms; omitting it is a usage error.
func TestGrantDeny_Dbms_MissingOnDbms_ReturnsUsageError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action create_role --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "requires --on-dbms")
		})
	}
}

// TestGrantDeny_FindAction_ReturnsUnknownActionError verifies that FIND (removed)
// is no longer accepted as a valid action.
func TestGrantDeny_FindAction_ReturnsUnknownActionError(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action find --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "unknown --action")
		})
	}
}

// TestGrant_DefaultGraphWhenNoResourceFlag_UsesWildcard verifies that READ
// (catPropertyBearer) with no resource flag defaults to ON GRAPH *.
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

// TestGrantDeny_DbmsActionWithOnGraph_RequiresOnDbms verifies that DBMS-level
// actions reject --on-graph (they require --on-dbms).
func TestGrantDeny_DbmsActionWithOnGraph_RequiresOnDbms(t *testing.T) {
	for _, sub := range []string{"grant", "deny"} {
		t.Run(sub, func(t *testing.T) {
			_, _, err := runMutation(t, sub, "--action create_role --on-graph '*' --role analyst", nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Message, "requires --on-dbms")
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
