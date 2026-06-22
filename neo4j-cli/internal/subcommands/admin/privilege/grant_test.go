// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sequencedCall struct {
	cypher string
	params map[string]any
}

// withRecordingSequencedExecFn replaces privilegeExecFn with a sequenced fake
// that records each call's cypher/params into calls and returns responses in
// order. It fails the test if called more times than there are responses.
func withRecordingSequencedExecFn(t *testing.T, calls *[]sequencedCall, responses []struct {
	rows []map[string]any
	err  error
}) {
	t.Helper()
	orig := privilegeExecFn
	idx := 0
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) {
		*calls = append(*calls, sequencedCall{cypher: cypher, params: params})
		if idx >= len(responses) {
			t.Fatalf("privilegeExecFn called %d times but only %d response(s) were provided", idx+1, len(responses))
		}
		r := responses[idx]
		idx++
		return r.rows, r.err
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}

// runGrant builds the `admin privilege grant` command tree, installs a
// recording sequenced exec-fn, then executes with args.
func runGrant(t *testing.T, args string, calls *[]sequencedCall, responses []struct {
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
	cmd.SetArgs(append([]string{"grant"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func twoOK() []struct {
	rows []map[string]any
	err  error
} {
	return []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},
		{rows: []map[string]any{}, err: nil},
	}
}

func TestGrant_PropertyBearer_GrantsAndShowsPrivileges(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action read --on-graph * --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT READ {*} ON GRAPH * ELEMENTS * TO $role", calls[0].cypher)
	assert.Equal(t, "analyst", calls[0].params["role"])
	assert.Equal(t, "SHOW ROLE $name PRIVILEGES", calls[1].cypher)
	assert.Equal(t, "analyst", calls[1].params["name"])
}

func TestGrant_GraphOnly_NoPropertyClause(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action traverse --on-graph * --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT TRAVERSE ON GRAPH * ELEMENTS * TO $role", calls[0].cypher)
}

func TestGrant_SetLabel_EmitsLabelClause(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action set_label --node-label Person --on-graph neo4j --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT SET LABEL `Person` ON GRAPH `neo4j` TO $role", calls[0].cypher)
}

func TestGrant_Database_EmitsDatabaseClause(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action access --on-database neo4j --role analyst", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT ACCESS ON DATABASE `neo4j` TO $role", calls[0].cypher)
}

func TestGrant_Dbms_EmitsDbmsClause(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action create_role --on-dbms --role admin", &calls, twoOK())
	require.NoError(t, err)

	require.Len(t, calls, 2)
	assert.Equal(t, "GRANT CREATE ROLE ON DBMS TO $role", calls[0].cypher)
	assert.Equal(t, "admin", calls[0].params["role"])
}

func TestGrant_MissingAction_ReturnsUsageError(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "--action is required")
	assert.Empty(t, calls, "seam must not be called when --action is missing")
}

func TestGrant_MissingRole_ReturnsUsageError(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action read", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "--role is required")
	assert.Empty(t, calls, "seam must not be called when --role is missing")
}

func TestGrant_FlagConflict_ReturnsUsageErrorWithoutSeam(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action traverse --property name --on-graph * --role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Empty(t, calls, "seam must not be called on a flag-conflict error")
}

func TestGrant_DbmsMissingOnDbms_ReturnsUsageError(t *testing.T) {
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action create_role --role analyst", &calls, nil)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Empty(t, calls, "seam must not be called when --on-dbms is missing for a DBMS action")
}

func TestGrant_MutationExecError_SkipsFollowUp(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action read --on-graph * --role analyst", &calls, []struct {
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

func TestGrant_FollowUpExecError_Propagates(t *testing.T) {
	followUpErr := clierr.NewValidationError("show role privileges failed")
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action read --on-graph * --role analyst", &calls, []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},
		{rows: nil, err: followUpErr},
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "show role privileges failed")
	require.Len(t, calls, 2)
}

func TestGrant_EnterpriseOnlyError_PropagatesValidationError(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("GRANT is not supported (requires Enterprise edition)")
	var calls []sequencedCall
	_, _, err := runGrant(t, "--action read --on-graph * --role analyst", &calls, []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: enterpriseErr},
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "requires Enterprise edition")
}
