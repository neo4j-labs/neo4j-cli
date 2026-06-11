// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildDropCmd(t *testing.T, stdin string) (*bytes.Buffer, *bytes.Buffer, func(args string) error) {
	t.Helper()

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)

	conn := testConn()
	run := func(args string) error {
		cmd := NewCmd(cfg, &conn, dbExecFn)
		flags.RegisterOutputFlag(cmd, cfg)
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
		Name:          "admin database drop",
		NoFlagsArgs:   "mydb",
		BothFlagsArgs: "mydb --yes --force",
		ResourceLabel: "database",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			var invoked bool
			orig := dbExecFn
			dbExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
				invoked = true
				return []map[string]any{}, nil
			}
			t.Cleanup(func() { dbExecFn = orig })

			_, errBuf, run := buildDropCmd(t, stdin)
			err := run(args)
			return confirmtest.GateRunResult{Err: err, Stderr: errBuf.String(), Invoked: invoked}
		},
	})
}

func TestDrop_YesForce_Succeeds(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	_, _, err := runDrop(t, "mydb --yes --force", "", []map[string]any{}, nil)
	require.NoError(t, err)
}

func TestDrop_ExecError_PropagatesError(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	execErr := clierr.NewValidationError("bolt connection refused")
	_, _, err := runDrop(t, "mydb --yes --force", "", nil, execErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestDrop_NotFound_ReturnsNotFoundError(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	notFoundErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Database.DatabaseNotFound",
		Msg:  "Database 'ghost' does not exist.",
	}
	_, _, err := runDrop(t, "ghost --yes --force", "", nil, notFoundErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Contains(t, ce.Message, `"ghost"`)
}

func TestDrop_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runDrop(t, "", "", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestDrop_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, dbExecFn)

	var found bool
	for _, c := range cmd.Commands() {
		if c.Name() == "drop" {
			assert.Equal(t, "true", c.Annotations["write"])
			found = true
			break
		}
	}
	require.True(t, found, "drop subcommand must be registered")
}
