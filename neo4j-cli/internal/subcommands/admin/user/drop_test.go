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
		cmd := NewCmd(cfg, &conn, userExecFn)
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

func runDrop(t *testing.T, args string, execErr error) (string, string, error) {
	t.Helper()

	orig := userExecFn
	userExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return nil, execErr
	}
	t.Cleanup(func() { userExecFn = orig })

	_, errBuf, run := buildDropCmd(t, "")
	execCmdErr := run(args)
	return "", errBuf.String(), execCmdErr
}

func TestDrop_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "admin user drop",
		NoFlagsArgs:   "alice",
		BothFlagsArgs: "alice --yes --force",
		ResourceLabel: "user",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			var invoked bool
			orig := userExecFn
			userExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
				invoked = true
				return []map[string]any{}, nil
			}
			t.Cleanup(func() { userExecFn = orig })

			_, errBuf, run := buildDropCmd(t, stdin)
			err := run(args)
			return confirmtest.GateRunResult{Err: err, Stderr: errBuf.String(), Invoked: invoked}
		},
	})
}

func TestDrop_YesForce_Succeeds(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	_, _, err := runDrop(t, "alice --yes --force", nil)
	require.NoError(t, err)
}

func TestDrop_ExecError_PropagatesError(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	execErr := clierr.NewValidationError("user not found")
	_, _, err := runDrop(t, "alice --yes --force", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "user not found")
}

func TestDrop_NotFound_ReturnsNotFoundError(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	notFoundErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Statement.ArgumentError",
		Msg:  "Failed to delete the specified user 'ghost': User does not exist.",
	}
	_, _, err := runDrop(t, "ghost --yes --force", notFoundErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Contains(t, ce.Message, `"ghost"`)
}

func TestDrop_InvalidArguments_DoesNotReturnNotFound(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	// The old wrong code; must NOT trigger not_found to prevent regression.
	invalidArgsErr := &neo4j.Neo4jError{
		Code: "Neo.ClientError.Security.InvalidArguments",
		Msg:  "Some other security error does not exist.",
	}
	_, _, err := runDrop(t, "ghost --yes --force", invalidArgsErr)
	require.Error(t, err)
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		assert.NotEqual(t, 3, ce.Code, "InvalidArguments must not produce not_found exit code")
	}
	// The raw Neo4jError is returned (not translated) confirming it was not matched
	// as a not-found condition.
	var ne *neo4j.Neo4jError
	require.True(t, errors.As(err, &ne), "expect the raw Neo4jError to propagate unchanged")
	assert.Equal(t, "Neo.ClientError.Security.InvalidArguments", ne.Code)
}

func TestDrop_WriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "drop" {
			assert.Equal(t, "true", sub.Annotations["write"], "drop must be annotated write=true")
			return
		}
	}
	t.Fatal("drop subcommand not found")
}
