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

// runCreate builds the `admin role create` command tree, installs the sequenced
// exec-fn (mutation call 1, follow-up SHOW call 2), then executes with args.
func runCreate(t *testing.T, args string, responses []struct {
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
	cmd.SetArgs(append([]string{"create"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestCreate_HappyPath_EmitsEmptyMemberList(t *testing.T) {
	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},                // CREATE ROLE
		{rows: []map[string]any{}, err: nil}, // SHOW ROLES WITH USERS WHERE role = $name
	}

	stdout, _, err := runCreate(t, "analyst --format json", responses)
	require.NoError(t, err)

	stdout = strings.TrimSpace(stdout)
	assert.Equal(t, "[]", stdout, "freshly created role should produce empty JSON array")
}

func TestCreate_HappyPath_EmitsMemberRows(t *testing.T) {
	memberRows := []map[string]any{
		{"role": "analyst", "member": "alice"},
	}
	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},        // CREATE ROLE
		{rows: memberRows, err: nil}, // SHOW ROLES WITH USERS WHERE role = $name
	}

	stdout, _, err := runCreate(t, "analyst --format json", responses)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "analyst", got[0]["role"])
	assert.Equal(t, "alice", got[0]["member"])
}

func TestCreate_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("bolt connection refused")

	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: execErr},
	}

	_, _, err := runCreate(t, "analyst", responses)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestCreate_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	// translateAdminError converts UnsupportedAdministrationCommand to a
	// validation error with the Enterprise edition hint before it reaches the
	// leaf. Simulate that translated error here.
	enterpriseErr := clierr.NewValidationError("CREATE ROLE is not supported (requires Enterprise edition)")

	responses := []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: enterpriseErr},
	}

	_, _, err := runCreate(t, "analyst", responses)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestCreate_NoArgs_CobraUsageError(t *testing.T) {
	responses := []struct {
		rows []map[string]any
		err  error
	}{}

	_, _, err := runCreate(t, "", responses)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
