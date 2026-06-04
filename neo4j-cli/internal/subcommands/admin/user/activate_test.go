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
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runActivate(t *testing.T, args string, execErr error) (string, string, string, map[string]any, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [{"name":"local","uri":"neo4j://localhost:7687","username":"neo4j","password":"pw","databaseName":"neo4j"}], "default-credential": "local"},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	credential := "local"
	captureFn, getCypher, getParams := captureExecFn(t, execErr)
	cmd := NewCmd(cfg, &credential, captureFn)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"activate"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), getCypher(), getParams(), execCmdErr
}

func TestActivate_HappyPath(t *testing.T) {
	_, _, cypher, params, err := runActivate(t, "alice", nil)
	require.NoError(t, err)

	assert.Equal(t, "ALTER USER $name SET STATUS ACTIVE", cypher)
	assert.Equal(t, "alice", params["name"])
}

func TestActivate_CommunityEditionError_PropagatesError(t *testing.T) {
	communityErr := clierr.NewValidationError("SET STATUS is not available in community edition")
	_, _, _, _, err := runActivate(t, "alice", communityErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "SET STATUS")
	assert.Contains(t, ce.Message, "community edition")
}

func TestActivate_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("user not found")
	_, _, _, _, err := runActivate(t, "alice", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "user not found")
}

func TestActivate_WriteAnnotation(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "activate" {
			assert.Equal(t, "true", sub.Annotations["write"], "activate must be annotated write=true")
			return
		}
	}
	t.Fatal("activate subcommand not found")
}

func TestActivate_NoArgs_CobraUsageError(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, nil))
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"activate"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, strings.ToLower(execErr.Error()), "accepts 1 arg")
}
