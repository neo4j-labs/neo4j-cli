// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"bytes"
	"errors"
	"sort"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCategory builds a category leaf via newCategoryCmd, mounts it under a stub
// parent so flag parsing matches the production tree, installs a recording
// sequenced exec-fn, then executes with args.
func runCategory(t *testing.T, verb string, cat actionCategory, args string, calls *[]sequencedCall, responses []struct {
	rows []map[string]any
	err  error
}) error {
	t.Helper()

	withRecordingSequencedExecFn(t, calls, responses)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()

	leaf := newCategoryCmd(cfg, &conn, verb, cat)
	parent := &cobra.Command{Use: "privilege"}
	parent.AddCommand(leaf)
	flags.RegisterOutputFlag(parent, cfg)

	parent.SetOut(bytes.NewBuffer(nil))
	parent.SetErr(bytes.NewBuffer(nil))

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	parent.SetArgs(append([]string{categoryMeta[cat].name}, argv...))

	return parent.Execute()
}

func TestNewCategoryCmd_Structure(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()

	cmd := newCategoryCmd(cfg, &conn, "GRANT", propertyBearer)

	assert.Equal(t, "property <action>", cmd.Use)
	assert.Equal(t, "true", cmd.Annotations["write"])
	assert.Equal(t, kebabActionsForCategory(propertyBearer), cmd.ValidArgs)

	// Only the property category flags + --role are registered (no --on-dbms etc.).
	var got []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) { got = append(got, f.Name) })
	sort.Strings(got)
	want := []string{"node-label", "on-graph", "property", "relationship-type", "role"}
	assert.Equal(t, want, got)

	assert.NotEmpty(t, cmd.Example)
	assert.NotContains(t, cmd.Long, "--action")
}

func TestNewCategoryCmd_RevokeRegistersRevokeType(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()

	cmd := newCategoryCmd(cfg, &conn, "REVOKE", propertyBearer)
	assert.NotNil(t, cmd.Flags().Lookup("revoke-type"))
}

func TestNewCategoryCmd_GrantEmitsSameCypherAsActionPath(t *testing.T) {
	var calls []sequencedCall
	err := runCategory(t, "GRANT", propertyBearer, "read --on-graph * --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT READ {*} ON GRAPH * ELEMENTS * TO $role", calls[0].cypher)
	assert.Equal(t, "analyst", calls[0].params["role"])
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", calls[1].cypher)
}

func TestNewCategoryCmd_KebabPositionalResolvesCanonical(t *testing.T) {
	var calls []sequencedCall
	err := runCategory(t, "GRANT", propertyBearer, "set-property --on-graph * --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT SET PROPERTY {*} ON GRAPH * ELEMENTS * TO $role", calls[0].cypher)
}

func TestNewCategoryCmd_RevokeResolvesVerb(t *testing.T) {
	var calls []sequencedCall
	err := runCategory(t, "REVOKE", propertyBearer, "read --on-graph * --role analyst --revoke-type grant", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "REVOKE GRANT READ {*} ON GRAPH * ELEMENTS * FROM $role", calls[0].cypher)
}

// TestCategoryCmd_PerCategoryCypherParity drives one representative action per
// category through the actual category subcommand and asserts the emitted Cypher
// matches the server-validated fragments (REQ-NF-005/007). The fragments below
// are server-validated — do not change them to invalid Cypher to make a test
// pass (see the note atop TestBuildPrivilegeCypher_HappyPaths).
func TestCategoryCmd_PerCategoryCypherParity(t *testing.T) {
	cases := []struct {
		name string
		cat  actionCategory
		args string
		want string
	}{
		{
			name: "propertyBearer",
			cat:  propertyBearer,
			args: "read --on-graph * --property name --role analyst",
			want: "GRANT READ {`name`} ON GRAPH * ELEMENTS * TO $role",
		},
		{
			name: "graphOnly entity",
			cat:  graphOnly,
			args: "traverse --on-graph * --node-label Person --role analyst",
			want: "GRANT TRAVERSE ON GRAPH * NODES `Person` TO $role",
		},
		{
			name: "graphWhole",
			cat:  graphWhole,
			args: "write --on-graph * --role analyst",
			want: "GRANT WRITE ON GRAPH * TO $role",
		},
		{
			name: "labelScoped multi label",
			cat:  labelScoped,
			args: "set-label --node-label Person --node-label Movie --on-graph neo4j --role analyst",
			want: "GRANT SET LABEL `Person`, `Movie` ON GRAPH `neo4j` TO $role",
		},
		{
			name: "load on cidr",
			cat:  load,
			args: "load --cidr 127.0.0.1/32 --role analyst",
			want: `GRANT LOAD ON CIDR "127.0.0.1/32" TO $role`,
		},
		{
			name: "database",
			cat:  database,
			args: "access --on-database neo4j --role analyst",
			want: "GRANT ACCESS ON DATABASE `neo4j` TO $role",
		},
		{
			name: "dbms",
			cat:  dbms,
			args: "create-role --role analyst",
			want: "GRANT CREATE ROLE ON DBMS TO $role",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []sequencedCall
			err := runCategory(t, "GRANT", tc.cat, tc.args, &calls, twoOK())
			require.NoError(t, err)
			require.Len(t, calls, 2)
			assert.Equal(t, tc.want, calls[0].cypher)
			assert.Equal(t, "analyst", calls[0].params["role"])
			assert.Equal(t, "SHOW ROLE $name PRIVILEGES", calls[1].cypher)
		})
	}
}

// TestCategoryCmd_CrossCategoryPositional_PerCategory asserts that invoking each
// category with an action belonging to a different category yields a usage error
// naming the correct category command (REQ-F-030).
func TestCategoryCmd_CrossCategoryPositional_PerCategory(t *testing.T) {
	cases := []struct {
		name     string
		on       actionCategory
		action   string
		wantHint string
	}{
		{name: "property given a database action", on: propertyBearer, action: "access", wantHint: "admin privilege grant database access"},
		{name: "database given a dbms action", on: database, action: "create-role", wantHint: "admin privilege grant dbms create-role"},
		{name: "graph given a property action", on: graphWhole, action: "read", wantHint: "admin privilege grant property read"},
		{name: "load given a graph action", on: load, action: "write", wantHint: "admin privilege grant graph write"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls []sequencedCall
			err := runCategory(t, "GRANT", tc.on, tc.action+" --role analyst", &calls, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 2, ce.Code)
			assert.Contains(t, ce.Message, tc.wantHint)
			assert.Empty(t, calls)
		})
	}
}

func TestNewCategoryCmd_UnknownAction_ReturnsUnknownActionError(t *testing.T) {
	var calls []sequencedCall
	err := runCategory(t, "GRANT", propertyBearer, "find --role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "unknown action")
	assert.Empty(t, calls)
}

func TestNewCategoryCmd_MissingRole_ReturnsUsageError(t *testing.T) {
	var calls []sequencedCall
	err := runCategory(t, "GRANT", propertyBearer, "read --on-graph *", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "--role is required")
	assert.Empty(t, calls)
}

func TestNewCategoryCmd_OnlyCategoryFlagsRegistered(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()

	cmd := newCategoryCmd(cfg, &conn, "GRANT", load)
	assert.Nil(t, cmd.Flags().Lookup("on-graph"))
	assert.Nil(t, cmd.Flags().Lookup("on-dbms"))
	assert.NotNil(t, cmd.Flags().Lookup("cidr"))
	assert.NotNil(t, cmd.Flags().Lookup("role"))
}
