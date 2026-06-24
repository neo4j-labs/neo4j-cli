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

func TestNewCategoryCmd_CrossCategoryPositional_NamesCorrectCommand(t *testing.T) {
	var calls []sequencedCall
	// "access" is a database privilege invoked on the property category.
	err := runCategory(t, "GRANT", propertyBearer, "access --role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "admin privilege grant database access")
	assert.Empty(t, calls)
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
