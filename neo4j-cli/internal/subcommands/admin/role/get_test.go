// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// withSequencedExecFn replaces roleExecFn for the duration of t with a fake
// that returns responses from the supplied slice in order (one per call).
func withSequencedExecFn(t *testing.T, responses []struct {
	rows []map[string]any
	err  error
}) {
	t.Helper()
	orig := roleExecFn
	idx := 0
	roleExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if idx >= len(responses) {
			t.Fatalf("roleExecFn called more times than expected (call %d)", idx+1)
		}
		r := responses[idx]
		idx++
		return r.rows, r.err
	}
	t.Cleanup(func() { roleExecFn = orig })
}

// runGet builds the `admin role get` command tree, installs a sequenced exec-fn,
// then executes the command with args.
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

// existsResponse returns a sequenced response pair for a role that exists.
// First call (SHOW ROLES WITH USERS WHERE role = $name) returns one row; second call
// (SHOW ROLE $name PRIVILEGES) returns the privilege rows.
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

// notFoundResponse returns responses for a role that does not exist.
// First call (SHOW ROLES WITH USERS WHERE role = $name) returns zero rows.
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
	privilegeRows := []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "admin"},
	}

	stdout, _, err := runGet(t, "admin --format json", existsResponse(privilegeRows))
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "admin", got[0]["role"])
}

func TestGet_HappyPath_FormatTable(t *testing.T) {
	privilegeRows := []map[string]any{
		{"access": "GRANTED", "action": "access", "resource": "database", "graph": "*", "segment": "database", "role": "admin"},
	}

	stdout, _, err := runGet(t, "admin --format table", existsResponse(privilegeRows))
	require.NoError(t, err)

	upper := strings.ToUpper(stdout)
	assert.Contains(t, upper, "ROLE")
	assert.Contains(t, stdout, "admin")
}

func TestGet_ExistingRole_NoPrivileges_ReturnsEmptyList(t *testing.T) {
	stdout, _, err := runGet(t, "emptyrole --format json", existsResponse([]map[string]any{}))
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Empty(t, got)
}

func TestGet_NotFound_ReturnsNotFoundError(t *testing.T) {
	stdout, _, err := runGet(t, "ghost --format json", notFoundResponse())
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code) // not_found
	assert.Contains(t, ce.Message, "ghost")
	assert.Empty(t, strings.TrimSpace(stdout))
}

func TestGet_ExecError_PropagatesError(t *testing.T) {
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

	withSequencedExecFn(t, responses)

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, roleExecFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"get", "--format", "json"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
