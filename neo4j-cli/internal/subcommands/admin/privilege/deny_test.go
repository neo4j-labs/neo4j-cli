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
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runDeny builds the `admin privilege deny` command tree, installs a recording
// sequenced exec-fn, then executes with args.
func runDeny(t *testing.T, args string, calls *[]sequencedCall, responses []struct {
	rows []map[string]any
	err  error
}) (string, string, error) {
	t.Helper()

	withRecordingSequencedExecFn(t, calls, responses)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, privilegeExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

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

func TestDeny_PropertyBearer_DeniesAndShowsPrivileges(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runDeny(t, "--action write --on-graph * --role readonly", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "DENY WRITE ON GRAPH * TO $role", calls[0].cypher)
	assert.Equal(t, "readonly", calls[0].params["role"])
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", calls[1].cypher)
	assert.Equal(t, "readonly", calls[1].params["name"])
}

func TestDeny_Dbms_EmitsDbmsClause(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runDeny(t, "--action create_role --on-dbms --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "DENY CREATE ROLE ON DBMS TO $role", calls[0].cypher)
	assert.Equal(t, "analyst", calls[0].params["role"])
}

func TestDeny_MissingAction_ReturnsUsageError(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runDeny(t, "--role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "--action is required")
	assert.Empty(t, calls, "seam must not be called when --action is missing")
}

func TestDeny_MissingRole_ReturnsUsageError(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runDeny(t, "--action read", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "--role is required")
	assert.Empty(t, calls, "seam must not be called when --role is missing")
}

func TestDeny_FlagConflict_ReturnsUsageErrorWithoutSeam(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runDeny(t, "--action traverse --property name --on-graph * --role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Empty(t, calls, "seam must not be called on a flag-conflict error")
}

func TestDeny_MutationExecError_SkipsFollowUp(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")
	var calls []sequencedCall
	_, _, err := runDeny(t, "--action write --on-graph * --role readonly", &calls, []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: execErr},
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
	require.Len(t, calls, 1, "follow-up SHOW must not run after a mutation error")
}
