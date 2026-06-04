// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildDropCmd returns a configured admin role drop command tree for use in tests.
// Callers are responsible for setting roleExecFn before calling Execute.
func buildDropCmd(t *testing.T, stdin string) (*bytes.Buffer, *bytes.Buffer, func(args string) error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [{"name":"local","uri":"neo4j://localhost:7687","username":"neo4j","password":"pw","databaseName":"neo4j"}], "default-credential": "local"},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)

	credential := "local"
	run := func(args string) error {
		cmd := NewCmd(cfg, &credential, roleExecFn)
		cmd.SetOut(out)
		cmd.SetErr(errBuf)
		cmd.SetIn(strings.NewReader(stdin))
		argv, splitErr := shlex.Split(args)
		require.NoError(t, splitErr)
		cmd.SetArgs(append([]string{"drop"}, argv...))
		return cmd.Execute()
	}
	return out, errBuf, run
}

func runDrop(t *testing.T, args string, stdin string, rows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, rows, execErr)
	_, errBuf, run := buildDropCmd(t, stdin)
	execCmdErr := run(args)
	return "", errBuf.String(), execCmdErr
}

func TestDrop_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "admin role drop",
		NoFlagsArgs:   "analyst",
		BothFlagsArgs: "analyst --yes --force",
		ResourceLabel: "role",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			var invoked bool
			orig := roleExecFn
			roleExecFn = func(_ context.Context, _ *clicfg.Config, _ *credentials.DbmsCredential, _ string, _ map[string]any) ([]map[string]any, error) {
				invoked = true
				return []map[string]any{}, nil
			}
			t.Cleanup(func() { roleExecFn = orig })

			_, errBuf, run := buildDropCmd(t, stdin)
			err := run(args)
			return confirmtest.GateRunResult{Err: err, Stderr: errBuf.String(), Invoked: invoked}
		},
	})
}

func TestDrop_YesForce_Succeeds(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	_, _, err := runDrop(t, "analyst --yes --force", "", []map[string]any{}, nil)
	require.NoError(t, err)
}

func TestDrop_CommunityEditionError_ReturnsEnterpriseHint(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	enterpriseErr := clierr.NewValidationError("this command is not supported (requires Enterprise edition)")

	_, _, err := runDrop(t, "analyst --yes --force", "", nil, enterpriseErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Enterprise edition")
}

func TestDrop_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runDrop(t, "", "", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}
