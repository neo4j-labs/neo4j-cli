// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package admin

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// runAdminWithArgs builds a full admin command tree (via NewCmd), installs the
// fakeQueryRunner so no real Bolt connection is opened, then executes with the
// supplied args. Returns stdout, stderr, and the execution error.
func runAdminWithArgs(t *testing.T, cfg *clicfg.Config, args []string) (string, string, error) {
	t.Helper()
	withFakeRunner(t, &fakeQueryRunner{rows: []map[string]any{}})

	admin := NewCmd(cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	admin.SetOut(out)
	admin.SetErr(errBuf)
	admin.SetArgs(args)

	err := admin.Execute()
	return out.String(), errBuf.String(), err
}

// TestAdminConn_PasswordPrompt_NonTTY_ReturnsUsageError verifies that when no
// --password is provided and stdin is not a TTY, the admin PersistentPreRunE
// returns a usage error before any leaf command runs.
func TestAdminConn_PasswordPrompt_NonTTY_ReturnsUsageError(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return false }
	t.Cleanup(func() { dbconn.StdinIsTTY = origIsTTY })

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	_, _, err := runAdminWithArgs(t, cfg, []string{"database", "list"})

	require.Error(t, err)
	// Check via errors.As for the typed error, falling back to message check.
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		assert.Contains(t, ce.Message, "--password is required")
	} else {
		assert.Contains(t, err.Error(), "--password is required")
	}
}

// TestAdminConn_PasswordPrompt_TTY_PromptsAndProceeds verifies that when no
// --password is provided but stdin is a TTY, the password is read via
// dbconn.PasswordReader and the command proceeds.
func TestAdminConn_PasswordPrompt_TTY_PromptsAndProceeds(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	origReader := dbconn.PasswordReader
	dbconn.StdinIsTTY = func() bool { return true }
	dbconn.PasswordReader = func() (string, error) { return "tty-password", nil }
	t.Cleanup(func() {
		dbconn.StdinIsTTY = origIsTTY
		dbconn.PasswordReader = origReader
	})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	_, _, err := runAdminWithArgs(t, cfg, []string{"database", "list"})

	// fakeQueryRunner returns empty rows; command exits 0.
	require.NoError(t, err)
}

// TestAdminConn_PasswordSupplied_SkipsPrompt verifies that when --password is
// supplied on the command line, the password prompt is never invoked.
func TestAdminConn_PasswordSupplied_SkipsPrompt(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	origReader := dbconn.PasswordReader
	dbconn.StdinIsTTY = func() bool { return false }
	dbconn.PasswordReader = func() (string, error) {
		t.Fatal("PasswordReader must not be called when --password is supplied")
		return "", nil
	}
	t.Cleanup(func() {
		dbconn.StdinIsTTY = origIsTTY
		dbconn.PasswordReader = origReader
	})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	_, _, err := runAdminWithArgs(t, cfg, []string{
		"database", "list",
		"--password=supplied-pw",
	})

	require.NoError(t, err)
}

// TestAdminConn_PasswordEnvVar_SkipsPrompt verifies that NEO4J_PASSWORD env var
// fills the password so the post-resolution prompt is never triggered. Under
// REQ-F-010 a DBMS env set must be complete, so uri/username are supplied via
// env alongside the password.
func TestAdminConn_PasswordEnvVar_SkipsPrompt(t *testing.T) {
	t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", "1")
	t.Setenv(dbconn.EnvURI, "neo4j://env-host:7687")
	t.Setenv(dbconn.EnvUsername, "env-user")
	t.Setenv(dbconn.EnvPassword, "env-password")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	origReader := dbconn.PasswordReader
	dbconn.StdinIsTTY = func() bool { return false }
	dbconn.PasswordReader = func() (string, error) {
		t.Fatal("PasswordReader must not be called when NEO4J_PASSWORD is set")
		return "", nil
	}
	t.Cleanup(func() {
		dbconn.StdinIsTTY = origIsTTY
		dbconn.PasswordReader = origReader
	})

	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	_, _, err := runAdminWithArgs(t, cfg, []string{"database", "list"})

	require.NoError(t, err)
}
