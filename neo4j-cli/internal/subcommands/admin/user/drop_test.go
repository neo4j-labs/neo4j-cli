// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/cobra"

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
		// Wrap drop in a "user" parent so confirm.Require can use the correct
		// resource type label ("user") in its prompt and error messages.
		parent := &cobra.Command{Use: "user"}
		drop := newDropCmd(cfg, &conn)
		flags.RegisterRwFlag(drop)
		parent.AddCommand(drop)
		parent.SetOut(out)
		parent.SetErr(errBuf)
		parent.SetIn(strings.NewReader(stdin))
		argv, splitErr := shlex.Split(args)
		require.NoError(t, splitErr)
		parent.SetArgs(append([]string{"drop"}, argv...))
		return parent.Execute()
	}
	return out, errBuf, run
}

func runDrop(t *testing.T, args string, stdin string, execErr error) (string, string, error) {
	t.Helper()

	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return []map[string]any{}, execErr
	}))
	out, errBuf, run := buildDropCmd(t, stdin)
	execCmdErr := run(args)
	return out.String(), errBuf.String(), execCmdErr
}

func TestDrop_YesForce_Succeeds_EmitsDroppedRecord(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	stdout, _, err := runDrop(t, "alice --rw --yes --force", "", nil)
	require.NoError(t, err)
	assert.Contains(t, stdout, "alice")
	assert.Contains(t, stdout, "dropped")
}

func TestDrop_NotFound_ReturnsNotFoundError(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	notFoundErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Statement.ArgumentError",
		Msg:  "Failed to delete the specified user 'ghost': User 'ghost' does not exist.",
	}
	_, _, err := runDrop(t, "ghost --rw --yes --force", "", notFoundErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code) // not_found
	assert.Contains(t, ce.Message, `"ghost"`)
}

func TestDrop_ArgumentError_NotExist_MsgVariant(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	notFoundErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Statement.ArgumentError",
		Msg:  "User 'bob' does not exist.",
	}
	_, _, err := runDrop(t, "bob --rw --yes --force", "", notFoundErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Contains(t, ce.Message, `"bob"`)
}

func TestDrop_ExecError_PropagatesError(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	execErr := clierr.NewValidationError("bolt connection refused")
	_, _, err := runDrop(t, "alice --rw --yes --force", "", execErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "bolt connection refused")
}

func TestDrop_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "admin user drop",
		NoFlagsArgs:   "alice --rw",
		BothFlagsArgs: "alice --rw --yes --force",
		ResourceLabel: "user",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			var invoked bool
			withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
				invoked = true
				return []map[string]any{}, nil
			}))
			_, errBuf, run := buildDropCmd(t, stdin)
			err := run(args)
			return confirmtest.GateRunResult{Err: err, Stderr: errBuf.String(), Invoked: invoked}
		},
	})
}

func TestDrop_NoArgs_CobraUsageError(t *testing.T) {
	_, _, err := runDrop(t, "--rw --yes --force", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestDrop_HasWriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := newDropCmd(cfg, &conn)
	assert.Equal(t, "true", cmd.Annotations["write"])
}
