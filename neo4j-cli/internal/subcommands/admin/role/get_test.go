// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGet builds the `admin role get` command tree, installs the sequenced
// exec-fn, then executes with the provided args.
func runGet(t *testing.T, args string, responses []struct {
	rows []map[string]any
	err  error
}) (string, string, error) {
	t.Helper()

	withSequencedExecFn(t, responses)

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
	cmd.SetArgs(append([]string{"get"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

// existsResponse builds the two-call sequenced response slice for a role that
// exists. privilegeRows is the result of the second call (SHOW ROLE … PRIVILEGES).
func existsResponse(privilegeRows []map[string]any) []struct {
	rows []map[string]any
	err  error
} {
	return []struct {
		rows []map[string]any
		err  error
	}{
		{rows: []map[string]any{{"role": "admin"}}, err: nil},
		{rows: privilegeRows, err: nil},
	}
}

// notFoundResponse builds the one-call sequenced response slice for a role that
// does not exist (SHOW ROLES WITH USERS WHERE role = $name returns zero rows).
func notFoundResponse() []struct {
	rows []map[string]any
	err  error
} {
	return []struct {
		rows []map[string]any
		err  error
	}{
		{rows: []map[string]any{}, err: nil},
	}
}

func TestGet_HappyPath_FormatJson(t *testing.T) {
	privRows := []map[string]any{
		{"access": "GRANTED", "action": "traverse", "resource": "graph", "segment": "NODE(*)", "role": "admin"},
	}

	stdout, _, err := runGet(t, "admin --format json", existsResponse(privRows))
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "GRANTED", got[0]["access"])
}

func TestGet_HappyPath_FormatTable(t *testing.T) {
	privRows := []map[string]any{
		{"access": "GRANTED", "action": "traverse", "resource": "graph", "segment": "NODE(*)", "role": "admin"},
	}

	stdout, _, err := runGet(t, "admin --format table", existsResponse(privRows))
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	for _, col := range []string{"ACCESS", "ACTION", "RESOURCE", "ROLE", "SEGMENT"} {
		assert.Contains(t, upper, col, "table missing column %q", col)
	}
}

func TestGet_ExistingRole_NoPrivileges_ReturnsEmptyList(t *testing.T) {
	stdout, _, err := runGet(t, "admin --format json", existsResponse(nil))
	require.NoError(t, err)

	stdout = strings.TrimSpace(stdout)
	assert.Equal(t, "[]", stdout, "empty privilege list should produce [] JSON array")
}

func TestGet_NotFound_ReturnsNotFoundError(t *testing.T) {
	stdout, _, err := runGet(t, "ghost --format json", notFoundResponse())
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Contains(t, ce.Message, "ghost")
	assert.Empty(t, strings.TrimSpace(stdout))
}

func TestGet_ExecError_OnExistenceCheck_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: execErr},
	}

	_, _, err := runGet(t, "admin --format json", responses)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestGet_NoArgs_CobraUsageError(t *testing.T) {
	responses := []struct {
		rows []map[string]any
		err  error
	}{}

	_, _, err := runGet(t, "--format json", responses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
