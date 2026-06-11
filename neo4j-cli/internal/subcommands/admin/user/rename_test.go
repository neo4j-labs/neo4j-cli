// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRename builds the `admin user rename` command tree and executes it.
func runRename(t *testing.T, args string, execErr error) (string, string, error) {
	t.Helper()

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, execErr))
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"rename"}, argv...))

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestRename_HappyPath(t *testing.T) {
	_, _, err := runRename(t, "alice --new-name alice2", nil)
	require.NoError(t, err)
}

func TestRename_AuraArgumentError_TranslatedToFriendlyMessage(t *testing.T) {
	auraErr := clierr.NewValidationError("renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)")
	_, _, err := runRename(t, "alice --new-name alice2", auraErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Aura")
	assert.Contains(t, ce.Message, "non-native authentication provider")
}

func TestRename_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("user alice not found")
	_, _, err := runRename(t, "alice --new-name alice2", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "alice not found")
}

func TestRename_TwoPositionalArgs_Fails(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, nil))
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"rename", "alice", "alice2"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "accepts 1 arg")
}

func TestRename_MissingNewName_Fails(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, nil))
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"rename", "alice"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "new-name")
}

func TestRename_WriteAnnotation(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewCmd(cfg, &conn, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "rename" {
			assert.Equal(t, "true", sub.Annotations["write"], "rename must be annotated write=true")
			return
		}
	}
	t.Fatal("rename subcommand not found")
}
