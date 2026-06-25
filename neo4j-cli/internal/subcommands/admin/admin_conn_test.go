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
	"github.com/neo4j/cli/test/utils/testfs"
)

// envAcceptEnvVars is the env var bound to the accept-env-vars config key.
const envAcceptEnvVars = "NEO4J_CLI_ACCEPT_ENV_VARS"

// storedDefaultCredJSON returns a credentials.json with a single dbms
// credential set as the default.
func storedDefaultCredJSON(uri, username, password, dbName string) string {
	return `{"dbms":{"default-credential":"mydb","credentials":[{"name":"mydb","username":"` +
		username + `","password":"` + password + `","database-name":"` + dbName +
		`","uri":"` + uri + `"}]}}`
}

// cfgWithCreds builds a config backed by an in-memory FS seeded with the
// supplied credentials JSON. Never uses afero.NewOsFs (would read the dev
// machine's real credentials).
func cfgWithCreds(t *testing.T, credsJSON string) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, credsJSON)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
}

// runAdminCapture runs the admin tree with a capturing fakeQueryRunner and
// returns the runner so tests can assert the resolved *dbconn.Conn. Callers
// must ensure a password is resolvable (via env or a stored credential) so the
// PersistentPreRunE prompt branch is not reached.
func runAdminCapture(t *testing.T, cfg *clicfg.Config, args []string) (*fakeQueryRunner, error) {
	t.Helper()
	fake := &fakeQueryRunner{rows: []map[string]any{}}
	withFakeRunner(t, fake)

	admin := NewCmd(cfg)
	admin.SetOut(bytes.NewBuffer(nil))
	admin.SetErr(bytes.NewBuffer(nil))
	admin.SetArgs(args)

	err := admin.Execute()
	return fake, err
}

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

// TestAdminEnvVars_HonouredWhenGateOn drives the full admin tree (user list)
// and asserts the connection env vars override the stored default credential
// only when accept-env-vars is enabled (parity with query, REQ-F-011/F-012).
func TestAdminEnvVars_HonouredWhenGateOn(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(dbconn.EnvURI, "neo4j://env-host:7687")
	t.Setenv(dbconn.EnvUsername, "env-user")
	t.Setenv(dbconn.EnvPassword, "env-pass")
	t.Setenv(dbconn.EnvDatabase, "should-be-ignored")
	t.Chdir(t.TempDir())

	cfg := cfgWithCreds(t, storedDefaultCredJSON(
		"neo4j://stored-host:7687", "stored-user", "stored-pass", "stored-db"))
	fake, err := runAdminCapture(t, cfg, []string{"user", "list"})

	require.NoError(t, err)
	require.NotNil(t, fake.lastConn)
	assert.Equal(t, "neo4j://env-host:7687", fake.lastConn.URI)
	assert.Equal(t, "env-user", fake.lastConn.Username)
	assert.Equal(t, "env-pass", fake.lastConn.Password)
	assert.Equal(t, "", fake.lastConn.Database, "NEO4J_DATABASE must not affect admin")
}

// TestAdminEnvVars_IgnoredWhenGateOff asserts that with accept-env-vars off the
// admin tree ignores the connection env vars entirely and uses the stored
// default credential (parity with query).
func TestAdminEnvVars_IgnoredWhenGateOff(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(dbconn.EnvURI, "neo4j://env-host:7687")
	t.Setenv(dbconn.EnvUsername, "env-user")
	t.Setenv(dbconn.EnvPassword, "env-pass")
	t.Setenv(dbconn.EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	cfg := cfgWithCreds(t, storedDefaultCredJSON(
		"neo4j://stored-host:7687", "stored-user", "stored-pass", "stored-db"))
	fake, err := runAdminCapture(t, cfg, []string{"user", "list"})

	require.NoError(t, err)
	require.NotNil(t, fake.lastConn)
	assert.Equal(t, "neo4j://stored-host:7687", fake.lastConn.URI)
	assert.Equal(t, "stored-user", fake.lastConn.Username)
	assert.Equal(t, "stored-pass", fake.lastConn.Password)
	assert.Equal(t, "", fake.lastConn.Database, "admin must never set Database")
}

// TestAdminEnvVars_DatabaseNeverConsulted asserts NEO4J_DATABASE is never read
// in admin mode (skipDatabase=true) regardless of the gate, even with a stored
// credential carrying a database name.
func TestAdminEnvVars_DatabaseNeverConsulted(t *testing.T) {
	for _, gate := range []string{"1", ""} {
		t.Run("gate="+gate, func(t *testing.T) {
			t.Setenv(envAcceptEnvVars, gate)
			t.Setenv(dbconn.EnvURI, "")
			t.Setenv(dbconn.EnvUsername, "")
			t.Setenv(dbconn.EnvPassword, "")
			t.Setenv(dbconn.EnvDatabase, "env-db")
			t.Chdir(t.TempDir())

			cfg := cfgWithCreds(t, storedDefaultCredJSON(
				"neo4j://stored-host:7687", "stored-user", "stored-pass", "stored-db"))
			fake, err := runAdminCapture(t, cfg, []string{"user", "list"})

			require.NoError(t, err)
			require.NotNil(t, fake.lastConn)
			assert.Equal(t, "", fake.lastConn.Database,
				"NEO4J_DATABASE must never populate the admin connection database")
		})
	}
}

// TestAdminEnvVars_PartialSetError asserts that a partial DBMS env set surfaces
// the same missing-variable usage error as query when the gate is on. With no
// stored credential this is the env-spec completeness error (env_spec.go),
// which names the missing NEO4J_* vars — identical to the query path.
func TestAdminEnvVars_PartialSetError(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(dbconn.EnvURI, "neo4j://env-host:7687")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg := cfgWithCreds(t, "{}")
	_, err := runAdminCapture(t, cfg, []string{"user", "list"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), dbconn.EnvUsername)
	assert.Contains(t, err.Error(), dbconn.EnvPassword)
}
