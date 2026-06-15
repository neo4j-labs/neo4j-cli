// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	. "github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/user"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRename builds the `admin user rename` command with a fake exec-fn that
// returns rows/execErr for both the RENAME statement and the follow-up SHOW
// USERS query, then executes it with args.
func runRename(t *testing.T, args string, showRows []map[string]any, execErr error) (string, string, error) {
	t.Helper()

	callCount := 0
	withFakeExecFn(t, fakeExecFn(func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		callCount++
		if execErr != nil {
			return nil, execErr
		}
		// First call: RENAME USER statement (returns no rows)
		// Second call: SHOW USERS WHERE user = $name (returns showRows)
		if callCount == 1 {
			return []map[string]any{}, nil
		}
		return showRows, nil
	}))

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	cmd := NewRenameCmdForTest(cfg, &conn)
	flags.RegisterOutputFlag(cmd, cfg)
	flags.RegisterRwFlag(cmd)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(argv)

	execCmdErr := cmd.Execute()
	return out.String(), errBuf.String(), execCmdErr
}

func TestUserRename_Success_EmitsRenamedUser(t *testing.T) {
	showRows := []map[string]any{
		{"user": "bob", "roles": []any{"admin"}, "passwordChangeRequired": false, "suspended": false},
	}

	stdout, _, err := runRename(t, "alice --new-name bob --rw --format json", showRows, nil)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, "bob", got["user"])
}

func TestUserRename_TwoPositionalArgs_ExactArgsError(t *testing.T) {
	_, _, err := runRename(t, "alice extra --new-name bob --rw", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestUserRename_ExecError_Propagates(t *testing.T) {
	execErr := clierr.NewValidationError("authentication provider apart from native")

	_, _, err := runRename(t, "alice --new-name bob --rw", nil, execErr)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, "authentication provider apart from native")
}

func TestUserRename_MissingNewNameFlag_Error(t *testing.T) {
	_, _, err := runRename(t, "alice --rw", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "new-name")
}
