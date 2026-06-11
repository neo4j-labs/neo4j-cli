// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
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

func runSuspend(t *testing.T, args string, execErr error) (string, string, string, map[string]any, error) {
	t.Helper()

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	captureFn, getCypher, getParams := captureExecFn(t, execErr)
	cmd := NewCmd(cfg, &conn, captureFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"suspend"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), getCypher(), getParams(), execCmdErr
}

func TestSuspend_HappyPath(t *testing.T) {
	_, _, cypher, params, err := runSuspend(t, "alice", nil)
	require.NoError(t, err)

	assert.Equal(t, "ALTER USER $name SET STATUS SUSPENDED", cypher)
	assert.Equal(t, "alice", params["name"])
}

func TestSuspend_CommunityEditionError_PropagatesError(t *testing.T) {
	communityErr := clierr.NewValidationError("SET STATUS is not available in community edition")
	_, _, _, _, err := runSuspend(t, "alice", communityErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "SET STATUS")
	assert.Contains(t, ce.Message, "community edition")
}

func TestSuspend_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("user not found")
	_, _, _, _, err := runSuspend(t, "alice", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "user not found")
}

func TestSuspend_WriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "suspend" {
			assert.Equal(t, "true", sub.Annotations["write"], "suspend must be annotated write=true")
			return
		}
	}
	t.Fatal("suspend subcommand not found")
}

func TestSuspend_NoArgs_CobraUsageError(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, nil))
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"suspend"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, strings.ToLower(execErr.Error()), "accepts 1 arg")
}
