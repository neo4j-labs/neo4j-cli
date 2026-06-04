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
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRename builds the `admin user rename` command tree and executes it.
func runRename(t *testing.T, args string, execErr error) (string, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [{"name":"local","uri":"neo4j://localhost:7687","username":"neo4j","password":"pw","databaseName":"neo4j"}], "default-credential": "local"},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, execErr))
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
	_, _, err := runRename(t, "alice alice2", nil)
	require.NoError(t, err)
}

func TestRename_AuraArgumentError_TranslatedToFriendlyMessage(t *testing.T) {
	auraErr := clierr.NewValidationError("renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)")
	_, _, err := runRename(t, "alice alice2", auraErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "Aura")
	assert.Contains(t, ce.Message, "non-native authentication provider")
}

func TestRename_ExecError_PropagatesError(t *testing.T) {
	execErr := clierr.NewValidationError("user alice not found")
	_, _, err := runRename(t, "alice alice2", execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "alice not found")
}

func TestRename_NoArgs_CobraUsageError(t *testing.T) {
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
	cmd.SetArgs([]string{"rename", "alice"})
	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "accepts 2 arg")
}

func TestRename_WriteAnnotation(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	credential := "local"
	cmd := NewCmd(cfg, &credential, fakeExecFn(t, nil, nil))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "rename" {
			assert.Equal(t, "true", sub.Annotations["write"], "rename must be annotated write=true")
			return
		}
	}
	t.Fatal("rename subcommand not found")
}
