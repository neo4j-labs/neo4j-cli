// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"bytes"
	"context"
	"encoding/json"
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

// withSequencedCreateFn installs a sequenced exec-fn for create tests.
// First call returns mutationRows/mutationErr; subsequent calls return followUpRows/nil.
func withSequencedCreateFn(t *testing.T, mutationRows []map[string]any, mutationErr error, followUpRows []map[string]any) {
	t.Helper()
	orig := roleExecFn
	callIdx := 0
	roleExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if callIdx == 0 {
			callIdx++
			return mutationRows, mutationErr
		}
		callIdx++
		return followUpRows, nil
	}
	t.Cleanup(func() { roleExecFn = orig })
}

// withSequencedCreateFnCapture installs a sequenced exec-fn and captures the
// cypher of the first (mutation) call into *capturedCypher.
func withSequencedCreateFnCapture(t *testing.T, capturedCypher *string, mutationRows []map[string]any, mutationErr error, followUpRows []map[string]any) {
	t.Helper()
	orig := roleExecFn
	callIdx := 0
	roleExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, _ map[string]any) ([]map[string]any, error) {
		if callIdx == 0 {
			*capturedCypher = cypher
			callIdx++
			return mutationRows, mutationErr
		}
		callIdx++
		return followUpRows, nil
	}
	t.Cleanup(func() { roleExecFn = orig })
}

func runCreate(t *testing.T, args string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withSequencedCreateFn(t, rows, execErr, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"create"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestCreate_HappyPath_Succeeds(t *testing.T) {
	_, _, err := runCreate(t, "analyst", []map[string]any{}, nil)
	require.NoError(t, err)
}

func TestCreate_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	_, _, err := runCreate(t, "analyst", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestCreate_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runCreate(t, "analyst", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestCreate_CypherUsesIfNotExists(t *testing.T) {
	var capturedCypher string
	withSequencedCreateFnCapture(t, &capturedCypher, []map[string]any{}, nil, []map[string]any{})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"create", "analyst"})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, capturedCypher, "IF NOT EXISTS", "CREATE ROLE cypher must include IF NOT EXISTS")
}

func TestCreate_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runCreate(t, "", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestCreate_EmitsFollowUpMemberRecord_EmptyWhenNoMembers(t *testing.T) {
	stdout, _, err := runCreate(t, "analyst --format json", []map[string]any{}, nil)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Empty(t, got)
}

func TestCreate_EmitsFollowUpMemberRecord_WithMembers(t *testing.T) {
	memberRows := []map[string]any{
		{"role": "analyst", "member": "alice"},
	}

	withSequencedCreateFn(t, []map[string]any{}, nil, memberRows)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"create", "analyst", "--format", "json"})
	require.NoError(t, cmd.Execute())

	var got []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "analyst", got[0]["role"])
}
