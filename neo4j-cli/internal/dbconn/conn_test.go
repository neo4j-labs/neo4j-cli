// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
)

// envAcceptEnvVars is the env var bound to the accept-env-vars config key. Tests
// set it to "1" to opt into reading NEO4J_* connection env vars.
const envAcceptEnvVars = "NEO4J_CLI_ACCEPT_ENV_VARS"

// newAdminCmd returns a cobra command with the same persistent flags that
// admin.go registers, wired to the supplied config. This lets tests call
// ResolveConn(cmd, cfg, true) without importing the admin package (which
// would create an import cycle).
func newAdminCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{Use: "admin"}
	cmd.PersistentFlags().String("uri", "", "")
	cmd.PersistentFlags().StringP("username", "u", "", "")
	cmd.PersistentFlags().StringP("password", "p", "", "")
	cmd.PersistentFlags().String("env", "", "")
	cmd.PersistentFlags().StringP("credential", "c", "", "")
	cmd.PersistentFlags().Bool("debug", false, "")
	return cmd
}

// newQueryCmd returns a cobra command with the same persistent flags that the
// query package registers (including --database), used to test
// ResolveConn(cmd, cfg, false) paths.
func newQueryCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{Use: "query"}
	cmd.PersistentFlags().String("uri", "", "")
	cmd.PersistentFlags().StringP("username", "u", "", "")
	cmd.PersistentFlags().StringP("password", "p", "", "")
	cmd.PersistentFlags().String("env", "", "")
	cmd.PersistentFlags().StringP("credential", "c", "", "")
	cmd.PersistentFlags().Bool("debug", false, "")
	cmd.PersistentFlags().String("database", "", "")
	return cmd
}

// newCfgWithCreds returns a config backed by an in-memory filesystem with the
// supplied credentials JSON seeded.
func newCfgWithCreds(t *testing.T, credsJSON string) (*clicfg.Config, afero.Fs) {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, credsJSON)
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", clicfg.GlobalScope), fs
}

// storedDefaultCredJSON returns a credentials.json with one dbms credential
// set as the default.
func storedDefaultCredJSON(uri, username, password, dbName string) string {
	return `{"dbms":{"default-credential":"mydb","credentials":[{"name":"mydb","username":"` +
		username + `","password":"` + password + `","database-name":"` + dbName +
		`","uri":"` + uri + `"}]}}`
}

// TestResolveConn_Admin_Defaults verifies that admin ResolveConn with no flags,
// env vars, .env file, or stored credential returns the built-in defaults
// (uri=neo4j://localhost:7687, username=neo4j) with an empty password and an
// empty database (skipDatabase=true).
func TestResolveConn_Admin_Defaults(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "someval") // must NOT affect admin
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, DefaultURI, conn.URI)
	assert.Equal(t, DefaultUsername, conn.Username)
	assert.Equal(t, "", conn.Password)
	assert.Equal(t, "", conn.Database, "admin must never set Database")
}

// TestResolveConn_Admin_NEO4J_DATABASE_Ignored verifies that even when
// NEO4J_DATABASE is set in the environment, admin ResolveConn does not
// populate conn.Database.
func TestResolveConn_Admin_NEO4J_DATABASE_Ignored(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://host:7687")
	t.Setenv(EnvUsername, "user")
	t.Setenv(EnvPassword, "pass")
	t.Setenv(EnvDatabase, "should-be-ignored")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "", conn.Database, "NEO4J_DATABASE must not affect admin connection")
	assert.Equal(t, "neo4j://host:7687", conn.URI)
}

// TestResolveConn_Admin_DirectFlags verifies that --uri, --username, and
// --password flags resolve correctly for admin (skipDatabase=true).
func TestResolveConn_Admin_DirectFlags(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://flag-host:7687",
		"--username=flag-user",
		"--password=flag-pass",
	}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://flag-host:7687", conn.URI)
	assert.Equal(t, "flag-user", conn.Username)
	assert.Equal(t, "flag-pass", conn.Password)
	assert.Equal(t, "", conn.Database)
}

// TestResolveConn_Admin_EnvVars verifies that NEO4J_URI, NEO4J_USERNAME, and
// NEO4J_PASSWORD environment variables resolve for admin (skipDatabase=true).
func TestResolveConn_Admin_EnvVars(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "env-user")
	t.Setenv(EnvPassword, "env-pass")
	t.Setenv(EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://env-host:7687", conn.URI)
	assert.Equal(t, "env-user", conn.Username)
	assert.Equal(t, "env-pass", conn.Password)
	assert.Equal(t, "", conn.Database)
}

// TestResolveConn_Admin_DotenvLoaded verifies that a .env file is loaded when
// no --credential flag is set and no OS env vars are present.
func TestResolveConn_Admin_DotenvLoaded(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")

	cfg, fs := newCfgWithCreds(t, "{}")
	require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"),
		[]byte("NEO4J_URI=neo4j://dotenv-host:7687\nNEO4J_USERNAME=dotenv-user\nNEO4J_PASSWORD=dotenv-pass\nNEO4J_DATABASE=dotenv-db\n"),
		0644))

	cmd := newAdminCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://dotenv-host:7687", conn.URI)
	assert.Equal(t, "dotenv-user", conn.Username)
	assert.Equal(t, "dotenv-pass", conn.Password)
	assert.Equal(t, "", conn.Database, "NEO4J_DATABASE from dotenv must not affect admin")
}

// TestResolveConn_Admin_ConflictFlagAndCredential verifies that combining --uri
// with --credential returns an error.
func TestResolveConn_Admin_ConflictFlagAndCredential(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://example:7687",
		"--credential=mydb",
	}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--credential")
	assert.Contains(t, err.Error(), "--uri")
}

// TestResolveConn_Admin_StoredDefaultCredential verifies that when no explicit
// flags or env vars are set the stored default dbms credential is used.
// skipDatabase=true controls the env/flag path; the persisted-credential
// path still carries the stored DatabaseName through conn.Database, but admin
// never consults conn.Database (it always targets the system database in the
// Bolt runner).
func TestResolveConn_Admin_StoredDefaultCredential(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newAdminCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://stored:7687", conn.URI)
	assert.Equal(t, "storedUser", conn.Username)
	assert.Equal(t, "storedPass", conn.Password)
}

// TestResolveConn_Admin_PartialOverrideErrors verifies that providing only
// some of the three required params (uri/username/password) when a stored
// credential exists returns an error.
func TestResolveConn_Admin_PartialOverrideErrors(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newAdminCmd(cfg)
	// Provide only --uri — partial override of a stored credential.
	require.NoError(t, cmd.ParseFlags([]string{"--uri=neo4j://override:7687"}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--uri/NEO4J_URI")
	assert.Contains(t, err.Error(), "--username/NEO4J_USERNAME")
	assert.Contains(t, err.Error(), "--password/NEO4J_PASSWORD")
	// Admin partial override message must NOT mention --database.
	assert.NotContains(t, err.Error(), "--database")
}

// TestResolveConn_Admin_AllThreeFlagsBypassStoredCredential verifies that
// providing all three of --uri/--username/--password bypasses the stored
// credential.
func TestResolveConn_Admin_AllThreeFlagsBypassStoredCredential(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://flag:7687",
		"--username=flagUser",
		"--password=flagPass",
	}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://flag:7687", conn.URI)
	assert.Equal(t, "flagUser", conn.Username)
	assert.Equal(t, "flagPass", conn.Password)
}

// TestResolveConn_Admin_NamedCredentialFlag verifies that --credential <name>
// resolves a named persisted credential. The --credential path carries through
// the stored DatabaseName in conn.Database; admin ignores conn.Database at
// runtime because boltAdminRunner always targets the system database.
func TestResolveConn_Admin_NamedCredentialFlag(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := `{"dbms":{"default-credential":"","credentials":[{"name":"myprod","username":"prodUser","password":"prodPass","database-name":"prodDB","uri":"neo4j://prod:7687"}]}}`
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=myprod"}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://prod:7687", conn.URI)
	assert.Equal(t, "prodUser", conn.Username)
	assert.Equal(t, "prodPass", conn.Password)
}

// TestResolveConn_Admin_UnknownCredential verifies a descriptive error for an
// unknown --credential value.
func TestResolveConn_Admin_UnknownCredential(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=nosuchcred"}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nosuchcred")
	assert.Contains(t, err.Error(), "credential dbms add")
}

// TestResolveConn_Admin_UserAgent verifies the user-agent is derived from the
// config version with the neo4j-cli/v prefix.
func TestResolveConn_Admin_UserAgent(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"1.2.3", "neo4j-cli/v1.2.3"},
		{"", "neo4j-cli/vdev"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			t.Setenv(EnvURI, "")
			t.Setenv(EnvUsername, "")
			t.Setenv(EnvPassword, "")
			t.Setenv(EnvDatabase, "")
			t.Chdir(t.TempDir())

			fs := afero.NewMemMapFs()
			cfg := clicfg.NewConfig(fs, tc.version, clicfg.GlobalScope)
			cmd := newAdminCmd(cfg)

			conn, err := ResolveConn(cmd, cfg, true)
			require.NoError(t, err)
			assert.Equal(t, tc.want, conn.UserAgent)
		})
	}
}

// TestResolveDebug_FlagAndEnvPrecedence locks the six debug precedence cases:
// explicit flag wins outright (so --debug=false beats NEO4J_DEBUG=1); when the
// flag is not set the env value is consulted with strict-`1` acceptance.
func TestResolveDebug_FlagAndEnvPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		flagArgs  []string
		envValue  string
		wantDebug bool
	}{
		{name: "flag on, env unset", flagArgs: []string{"--debug"}, envValue: "", wantDebug: true},
		{name: "env=1, no flag", flagArgs: nil, envValue: "1", wantDebug: true},
		{name: "flag on, env=1", flagArgs: []string{"--debug"}, envValue: "1", wantDebug: true},
		{name: "both off", flagArgs: nil, envValue: "", wantDebug: false},
		{name: "env=true (not '1') leaves debug off", flagArgs: nil, envValue: "true", wantDebug: false},
		{name: "explicit --debug=false overrides env=1", flagArgs: []string{"--debug=false"}, envValue: "1", wantDebug: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NEO4J_DEBUG", tc.envValue)
			fs := afero.NewMemMapFs()
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			cmd := newAdminCmd(cfg)
			require.NoError(t, cmd.ParseFlags(tc.flagArgs))
			assert.Equal(t, tc.wantDebug, ResolveDebug(cmd))
		})
	}
}

// TestResolveConn_Admin_DebugFlagWiresToConn verifies that --debug propagates
// to conn.Debug so callers can pass it to the Bolt driver.
func TestResolveConn_Admin_DebugFlagWiresToConn(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--debug"}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.True(t, conn.Debug)
}

// TestResolveConn_Admin_NoCleartext_DebugOff verifies debug defaults to false
// when neither flag nor env is set.
func TestResolveConn_Admin_NoCleartext_DebugOff(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Setenv("NEO4J_DEBUG", "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.False(t, conn.Debug)
}

// TestResolveConn_Query_IncludesDatabase verifies that when skipDatabase=false
// (query mode) NEO4J_DATABASE is respected.
func TestResolveConn_Query_IncludesDatabase(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://host:7687")
	t.Setenv(EnvUsername, "user")
	t.Setenv(EnvPassword, "pass")
	t.Setenv(EnvDatabase, "mydb")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "mydb", conn.Database)
}

// TestResolveConn_Query_EmptyDatabaseWhenNotProvided verifies that when
// skipDatabase=false and no database source (dotenv, env, flag) provides a
// value, conn.Database is left empty so the server resolves the home database.
func TestResolveConn_Query_EmptyDatabaseWhenNotProvided(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://host:7687")
	t.Setenv(EnvUsername, "user")
	t.Setenv(EnvPassword, "pass")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "", conn.Database, "database must remain empty so server resolves home database")
}

// TestResolveConn_Query_CredentialPath_DatabaseOverride verifies that when
// --credential is used with --database, the flag overrides the stored
// credential's database name.
func TestResolveConn_Query_CredentialPath_DatabaseOverride(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := `{"dbms":{"default-credential":"","credentials":[{"name":"myprod","username":"prodUser","password":"prodPass","database-name":"prodDB","uri":"neo4j://prod:7687"}]}}`
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=myprod", "--database=override-db"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "override-db", conn.Database, "--database flag must override credential-supplied database")
}

// TestResolveConn_EnvGate_OffIgnoresEnvVars verifies that with accept-env-vars
// unset (default) the NEO4J_URI/USERNAME/PASSWORD/DATABASE env vars are ignored
// and the stored default credential is used instead.
func TestResolveConn_EnvGate_OffIgnoresEnvVars(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "env-user")
	t.Setenv(EnvPassword, "env-pass")
	t.Setenv(EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://stored:7687", conn.URI, "env vars must be ignored when accept-env-vars is off")
	assert.Equal(t, "storedUser", conn.Username)
	assert.Equal(t, "storedPass", conn.Password)
	assert.Equal(t, "storedDB", conn.Database)
}

// TestResolveConn_EnvGate_OnOverridesStoredCredential verifies that with
// accept-env-vars enabled the NEO4J_* env vars override the stored default
// credential at the documented precedence.
func TestResolveConn_EnvGate_OnOverridesStoredCredential(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "env-user")
	t.Setenv(EnvPassword, "env-pass")
	t.Setenv(EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://env-host:7687", conn.URI)
	assert.Equal(t, "env-user", conn.Username)
	assert.Equal(t, "env-pass", conn.Password)
	assert.Equal(t, "env-db", conn.Database)
}

// TestResolveConn_EnvGate_OffIgnoresNEO4J_DATABASEOnCredentialPath verifies the
// CLI-212 path (REQ-F-013): with accept-env-vars off, NEO4J_DATABASE no longer
// overrides a --credential credential's database, but --database still does.
func TestResolveConn_EnvGate_OffIgnoresNEO4J_DATABASEOnCredentialPath(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	credsJSON := `{"dbms":{"default-credential":"","credentials":[{"name":"myprod","username":"prodUser","password":"prodPass","database-name":"prodDB","uri":"neo4j://prod:7687"}]}}`
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=myprod"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "prodDB", conn.Database, "NEO4J_DATABASE must not override credential database when accept-env-vars is off")
}

// TestResolveConn_EnvGate_OnAppliesNEO4J_DATABASEOnCredentialPath verifies that
// with accept-env-vars on, NEO4J_DATABASE overrides a --credential credential's
// database (the CLI-212 behaviour, now gated).
func TestResolveConn_EnvGate_OnAppliesNEO4J_DATABASEOnCredentialPath(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	credsJSON := `{"dbms":{"default-credential":"","credentials":[{"name":"myprod","username":"prodUser","password":"prodPass","database-name":"prodDB","uri":"neo4j://prod:7687"}]}}`
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=myprod"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "env-db", conn.Database, "NEO4J_DATABASE must override credential database when accept-env-vars is on")
}

// TestResolveConn_EnvGate_FlagDatabaseOverridesRegardlessOfGate verifies that
// the explicit --database flag overrides a credential's database even when
// accept-env-vars is off (flags are never gated).
func TestResolveConn_EnvGate_FlagDatabaseOverridesRegardlessOfGate(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(EnvDatabase, "env-db")
	t.Chdir(t.TempDir())

	credsJSON := `{"dbms":{"default-credential":"","credentials":[{"name":"myprod","username":"prodUser","password":"prodPass","database-name":"prodDB","uri":"neo4j://prod:7687"}]}}`
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=myprod", "--database=flag-db"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "flag-db", conn.Database, "--database flag must override regardless of accept-env-vars")
}

// TestResolveConn_EnvGate_DotenvReadRegardlessOfGate verifies that the dotenv
// (--env walk-up) mechanism is unaffected by accept-env-vars: with the gate off
// a .env file still supplies connection values.
func TestResolveConn_EnvGate_DotenvReadRegardlessOfGate(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")

	cfg, fs := newCfgWithCreds(t, "{}")
	require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"),
		[]byte("NEO4J_URI=neo4j://dotenv-host:7687\nNEO4J_USERNAME=dotenv-user\nNEO4J_PASSWORD=dotenv-pass\nNEO4J_DATABASE=dotenv-db\n"),
		0644))

	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://dotenv-host:7687", conn.URI, "dotenv must be read regardless of accept-env-vars")
	assert.Equal(t, "dotenv-user", conn.Username)
	assert.Equal(t, "dotenv-pass", conn.Password)
	assert.Equal(t, "dotenv-db", conn.Database)
}

// TestResolveConn_EnvGate_PartialDBMSSetErrorsNamingVars verifies that with
// accept-env-vars on and only NEO4J_URI set (no USERNAME/PASSWORD), resolution
// fails with an error naming the missing NEO4J_* variables (REQ-F-010).
func TestResolveConn_EnvGate_PartialDBMSSetErrorsNamingVars(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)

	_, err := ResolveConn(cmd, cfg, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), EnvURI)
	assert.Contains(t, err.Error(), EnvUsername)
	assert.Contains(t, err.Error(), EnvPassword)
	// Env-mode message names the NEO4J_* vars, not the --flag/ENV dual form.
	assert.NotContains(t, err.Error(), "--uri")
}

// TestResolveConn_EnvGate_PartialDBMSSetErrorsAdminAndQuery verifies that both
// query (skipDatabase=false) and admin (skipDatabase=true) surface the same
// missing-variable error for a partial DBMS env set.
func TestResolveConn_EnvGate_PartialDBMSSetErrorsAdminAndQuery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		skipDatabase bool
		newCmd       func(*clicfg.Config) *cobra.Command
	}{
		{"query", false, newQueryCmd},
		{"admin", true, newAdminCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envAcceptEnvVars, "1")
			t.Setenv(EnvURI, "neo4j://env-host:7687")
			t.Setenv(EnvUsername, "env-user")
			t.Setenv(EnvPassword, "")
			t.Setenv(EnvDatabase, "")
			t.Chdir(t.TempDir())

			cfg, _ := newCfgWithCreds(t, "{}")
			cmd := tc.newCmd(cfg)

			_, err := ResolveConn(cmd, cfg, tc.skipDatabase)
			require.Error(t, err)
			assert.Contains(t, err.Error(), EnvPassword)
		})
	}
}

// TestResolveConn_EnvGate_CompleteDBMSSetNoError verifies that a complete DBMS
// env set passes the completeness check.
func TestResolveConn_EnvGate_CompleteDBMSSetNoError(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "env-user")
	t.Setenv(EnvPassword, "env-pass")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://env-host:7687", conn.URI)
}

// TestResolveConn_EnvGate_NEO4J_DATABASEAloneNoError verifies that
// NEO4J_DATABASE alone (no uri/user/pass) does not trigger a completeness
// error: it is an OptionalVar, not part of the required group.
func TestResolveConn_EnvGate_NEO4J_DATABASEAloneNoError(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "only-db")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "only-db", conn.Database)
}

// TestResolveConn_EnvGate_OffNoCompletenessError verifies that with
// accept-env-vars off a partial DBMS env set does not trigger a completeness
// error (the gate is closed, so env vars are ignored entirely).
func TestResolveConn_EnvGate_OffNoCompletenessError(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, DefaultURI, conn.URI, "partial env set must be ignored when accept-env-vars is off")
}

// TestResolveConn_Query_ThreeEnvVarsBypassStoredCredential verifies REQ-F-014:
// with accept-env-vars on, a stored default credential, and only the three
// required env vars (no NEO4J_DATABASE), the env vars override the stored
// credential without an "all four required" partial error.
func TestResolveConn_Query_ThreeEnvVarsBypassStoredCredential(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "env-user")
	t.Setenv(EnvPassword, "env-pass")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://env-host:7687", conn.URI)
	assert.Equal(t, "env-user", conn.Username)
	assert.Equal(t, "env-pass", conn.Password)
	assert.Equal(t, "", conn.Database, "database is optional; left empty so the server resolves the home database")
}

// TestResolveConn_Query_ThreeFlagsBypassStoredCredential verifies REQ-F-014 for
// flags: --uri/--username/--password (no --database) override a stored default
// credential without an "all four required" error.
func TestResolveConn_Query_ThreeFlagsBypassStoredCredential(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://flag:7687",
		"--username=flagUser",
		"--password=flagPass",
	}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://flag:7687", conn.URI)
	assert.Equal(t, "flagUser", conn.Username)
	assert.Equal(t, "flagPass", conn.Password)
	assert.Equal(t, "", conn.Database, "database optional; not pulled from the bypassed stored credential")
}

// TestResolveConn_Query_DatabaseAppliedButOptional verifies REQ-F-014: when
// --database is supplied alongside the three required flags it is applied, but
// it was never required to complete the override.
func TestResolveConn_Query_DatabaseAppliedButOptional(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://flag:7687",
		"--username=flagUser",
		"--password=flagPass",
		"--database=flag-db",
	}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://flag:7687", conn.URI)
	assert.Equal(t, "flag-db", conn.Database, "supplied database must be applied")
}

// TestResolveConn_Query_PartialFlagOverrideRejected verifies REQ-F-014: with a
// stored default credential, supplying one or two of uri/username/password via
// flags (no env) is still a rejected partial override naming the --flags.
func TestResolveConn_Query_PartialFlagOverrideRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"only uri", []string{"--uri=neo4j://flag:7687"}},
		{"uri+username", []string{"--uri=neo4j://flag:7687", "--username=flagUser"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvURI, "")
			t.Setenv(EnvUsername, "")
			t.Setenv(EnvPassword, "")
			t.Setenv(EnvDatabase, "")
			t.Chdir(t.TempDir())

			credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
			cfg, _ := newCfgWithCreds(t, credsJSON)
			cmd := newQueryCmd(cfg)
			require.NoError(t, cmd.ParseFlags(tc.args))

			_, err := ResolveConn(cmd, cfg, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "partial connection params")
			assert.Contains(t, err.Error(), "--uri")
			assert.NotContains(t, err.Error(), "all four", "the override is three required params, database optional")
		})
	}
}

// TestResolveConn_Query_PartialEnvOverrideRejectedNamingVars verifies REQ-F-014:
// with a stored default credential and a partial env set (two of three), the
// env-mode error names the missing NEO4J_* vars rather than the --flag form.
func TestResolveConn_Query_PartialEnvOverrideRejectedNamingVars(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "env-user")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)

	_, err := ResolveConn(cmd, cfg, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), EnvPassword)
	assert.NotContains(t, err.Error(), "--uri", "env-mode partial error names NEO4J_* vars, not --flags")
}

// TestResolveConn_Query_ThreeFlagsNoStoredCredNoRegression verifies REQ-F-014:
// with no stored credential, a three-flag override still resolves cleanly (no
// regression from the relaxed completeness rule).
func TestResolveConn_Query_ThreeFlagsNoStoredCredNoRegression(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://flag:7687",
		"--username=flagUser",
		"--password=flagPass",
	}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://flag:7687", conn.URI)
	assert.Equal(t, "flagUser", conn.Username)
	assert.Equal(t, "flagPass", conn.Password)
}

// TestResolveConn_Query_DatabaseOnlyOverrideLayersOntoStoredCredential verifies
// REQ-F-014: with a stored default credential and only --database supplied (none
// of the three required params), the database is no longer a partial override —
// it layers onto the stored credential's uri/username/password.
func TestResolveConn_Query_DatabaseOnlyOverrideLayersOntoStoredCredential(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedDefaultCredJSON("neo4j://stored:7687", "storedUser", "storedPass", "storedDB")
	cfg, _ := newCfgWithCreds(t, credsJSON)
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--database=flag-db"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://stored:7687", conn.URI)
	assert.Equal(t, "storedUser", conn.Username)
	assert.Equal(t, "storedPass", conn.Password)
	assert.Equal(t, "flag-db", conn.Database, "supplied database overrides the stored credential's database")
}

// TestLoadEnvFile_NoEnvReturnsEmpty verifies that when no .env is present the
// returned map is empty (not nil).
func TestLoadEnvFile_NoEnvReturnsEmpty(t *testing.T) {
	fs := afero.NewMemMapFs()
	got, err := LoadEnvFile(fs, "", "/some/dir", nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{}, got)
}

// TestLoadEnvFile_ExplicitPathLoaded verifies that an explicit --env path is
// loaded instead of the walk-up .env file.
func TestLoadEnvFile_ExplicitPathLoaded(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/explicit/custom.env",
		[]byte("NEO4J_PASSWORD=fromexplicit\n"), 0644))
	// Also seed a .env in cwd that must NOT be picked up.
	require.NoError(t, afero.WriteFile(fs, "/cwd/.env",
		[]byte("NEO4J_PASSWORD=fromcwd\n"), 0644))

	got, err := LoadEnvFile(fs, "/explicit/custom.env", "/cwd", nil)
	require.NoError(t, err)
	assert.Equal(t, "fromexplicit", got["NEO4J_PASSWORD"])
}

// TestLoadEnvFile_ExplicitMissingErrors verifies that a missing --env path
// returns an error containing the path.
func TestLoadEnvFile_ExplicitMissingErrors(t *testing.T) {
	fs := afero.NewMemMapFs()
	_, err := LoadEnvFile(fs, "/no/such/file", "/cwd", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/no/such/file")
}

// TestLoadEnvFile_WalkUpFindsParent verifies the walk-up behaviour finds a
// .env in a parent directory when not present in the start dir.
func TestLoadEnvFile_WalkUpFindsParent(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/work/.env",
		[]byte("NEO4J_URI=neo4j://walkup:7687\nNEO4J_USERNAME=walker\n"), 0644))
	deep := filepath.Join("/work", "deep", "nested")
	require.NoError(t, fs.MkdirAll(deep, 0755))

	got, err := LoadEnvFile(fs, "", deep, nil)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://walkup:7687", got[EnvURI])
	assert.Equal(t, "walker", got[EnvUsername])
}

// TestLoadEnvFile_AnnouncesAboveCwdInfoLine verifies that an info: line is
// emitted to stderr when the .env was found above the start dir.
func TestLoadEnvFile_AnnouncesAboveCwdInfoLine(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/proj/.env",
		[]byte("NEO4J_USERNAME=walker\n"), 0644))

	var buf bytes.Buffer
	_, err := LoadEnvFile(fs, "", "/proj/sub", &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "info: loading .env from")
}

// TestOverlay_LastNonEmptyWins verifies the Overlay helper returns the last
// non-empty value.
func TestOverlay_LastNonEmptyWins(t *testing.T) {
	assert.Equal(t, "c", Overlay("a", "", "c"))
	assert.Equal(t, "a", Overlay("a", "", ""))
	assert.Equal(t, "", Overlay("", "", ""))
	assert.Equal(t, "b", Overlay("", "b", ""))
}

// TestNormalizeURI_HTTPRewrite verifies that HTTP and HTTPS URIs are rewritten
// to Bolt equivalents.
func TestNormalizeURI_HTTPRewrite(t *testing.T) {
	rewritten, didRewrite, _, _ := NormalizeURI("http://host:7474")
	assert.True(t, didRewrite)
	assert.Equal(t, "neo4j://host:7687", rewritten)

	rewritten, didRewrite, _, _ = NormalizeURI("https://host:7473")
	assert.True(t, didRewrite)
	assert.Equal(t, "neo4j+s://host:7687", rewritten)
}

// TestNormalizeURI_BoltPassthrough verifies that bolt:// URIs pass through
// unchanged.
func TestNormalizeURI_BoltPassthrough(t *testing.T) {
	raw := "bolt://host:7687"
	rewritten, didRewrite, _, _ := NormalizeURI(raw)
	assert.False(t, didRewrite)
	assert.Equal(t, raw, rewritten)
}

// TestResolveConn_Admin_FlagPrecedenceOverEnv verifies that explicit flags beat
// environment variables for admin.
func TestResolveConn_Admin_FlagPrecedenceOverEnv(t *testing.T) {
	t.Setenv(envAcceptEnvVars, "1")
	t.Setenv(EnvURI, "neo4j://env-host:7687")
	t.Setenv(EnvUsername, "envUser")
	t.Setenv(EnvPassword, "envPass")
	t.Setenv(EnvDatabase, "envDB")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=neo4j://flag-host:7687",
		"--username=flagUser",
		"--password=flagPass",
	}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://flag-host:7687", conn.URI)
	assert.Equal(t, "flagUser", conn.Username)
	assert.Equal(t, "flagPass", conn.Password)
	assert.Equal(t, "", conn.Database)
}

// TestFlagString_ReturnsEmpty verifies FlagString returns an empty string when
// the flag is absent.
func TestFlagString_ReturnsEmpty(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	assert.Equal(t, "", FlagString(cmd, "missing"))
}

// TestFlagString_ReturnsValue verifies FlagString returns a registered flag's
// value.
func TestFlagString_ReturnsValue(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("mykey", "defval", "")
	assert.Equal(t, "defval", FlagString(cmd, "mykey"))
}

// TestPromptPassword_NonTTY_ReturnsUsageError verifies that PromptPassword
// returns a usage error when stdin is not a TTY.
func TestPromptPassword_NonTTY_ReturnsUsageError(t *testing.T) {
	origIsTTY := StdinIsTTY
	StdinIsTTY = func() bool { return false }
	t.Cleanup(func() { StdinIsTTY = origIsTTY })

	cmd := &cobra.Command{Use: "test"}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	_, err := PromptPassword(cmd, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "password is required")
	assert.Contains(t, err.Error(), "accept-env-vars")
	assert.NotContains(t, err.Error(), "set --password, NEO4J_PASSWORD")
}

// TestPasswordRequiredMessage_OnVsOff verifies the gated env var is advertised
// only when accept-env-vars is on.
func TestPasswordRequiredMessage_OnVsOff(t *testing.T) {
	off := PasswordRequiredMessage(false)
	assert.NotContains(t, off, "set --password, NEO4J_PASSWORD")
	assert.Contains(t, off, "--password")
	assert.Contains(t, off, ".env")
	assert.Contains(t, off, "accept-env-vars")

	on := PasswordRequiredMessage(true)
	assert.Contains(t, on, "NEO4J_PASSWORD")
}

// TestPromptPassword_TTY_ReadsFromPasswordReader verifies that PromptPassword
// reads from the PasswordReader seam on a TTY.
func TestPromptPassword_TTY_ReadsFromPasswordReader(t *testing.T) {
	origIsTTY := StdinIsTTY
	origReader := PasswordReader
	StdinIsTTY = func() bool { return true }
	PasswordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() {
		StdinIsTTY = origIsTTY
		PasswordReader = origReader
	})

	cmd := &cobra.Command{Use: "test"}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	pw, err := PromptPassword(cmd, false)
	require.NoError(t, err)
	assert.Equal(t, "prompted-pw", pw)
	assert.Contains(t, buf.String(), "Password: ")
}

// TestPromptPassword_TTY_ReaderError_Propagates verifies that a PasswordReader
// error is returned by PromptPassword.
func TestPromptPassword_TTY_ReaderError_Propagates(t *testing.T) {
	origIsTTY := StdinIsTTY
	origReader := PasswordReader
	StdinIsTTY = func() bool { return true }
	PasswordReader = func() (string, error) { return "", fmt.Errorf("read interrupted") }
	t.Cleanup(func() {
		StdinIsTTY = origIsTTY
		PasswordReader = origReader
	})

	cmd := &cobra.Command{Use: "test"}
	var buf bytes.Buffer
	cmd.SetErr(&buf)

	_, err := PromptPassword(cmd, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read interrupted")
}

// TestResolveConn_Admin_EnvPath_ExplicitFile verifies that --env pointing at an
// explicit dotenv file loads it instead of the walk-up behaviour.
func TestResolveConn_Admin_EnvPath_ExplicitFile(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, fs := newCfgWithCreds(t, "{}")
	require.NoError(t, afero.WriteFile(fs, "/custom/.env",
		[]byte("NEO4J_URI=neo4j://envfile-host:7687\nNEO4J_USERNAME=envfile-user\nNEO4J_PASSWORD=envfile-pass\n"),
		0644))

	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--env=/custom/.env"}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://envfile-host:7687", conn.URI)
	assert.Equal(t, "envfile-user", conn.Username)
	assert.Equal(t, "envfile-pass", conn.Password)
}

// TestResolveConn_Admin_ConflictPasswordAndCredential verifies that --password
// and --credential together return an error.
func TestResolveConn_Admin_ConflictPasswordAndCredential(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--password=mypass",
		"--credential=mydb",
	}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--credential")
	assert.Contains(t, err.Error(), "--password")
}
