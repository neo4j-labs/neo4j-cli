// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
)

// newTestCmd returns a fresh query parent command + a memfs config wired in,
// ready to have flags set on it. Tests reuse this rather than going through
// the full app.NewCmd tree.
func newTestCmd(t *testing.T) (*cobra.Command, *clicfg.Config) {
	t.Helper()
	fs := afero.NewMemMapFs()
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)
	return cmd, cfg
}

func TestLoadEnvFile_NoEnvReturnsEmpty(t *testing.T) {
	fs := afero.NewMemMapFs()
	got, err := loadEnvFile(fs, "", "/some/dir")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{}, got)
}

func TestLoadEnvFile_WalkUpFindsParent(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/work/.env",
		[]byte("NEO4J_URI=http://walkup:7474\nNEO4J_USERNAME=walker\n"), 0644))
	deep := filepath.Join("/work", "deep", "nested")
	require.NoError(t, fs.MkdirAll(deep, 0755))

	got, err := loadEnvFile(fs, "", deep)
	require.NoError(t, err)
	assert.Equal(t, "http://walkup:7474", got["NEO4J_URI"])
	assert.Equal(t, "walker", got["NEO4J_USERNAME"])
}

func TestLoadEnvFile_ExplicitPathShortCircuits(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/elsewhere/custom.env",
		[]byte("NEO4J_PASSWORD=fromfile\n"), 0644))
	// Also drop a .env in cwd that should NOT be picked up.
	require.NoError(t, afero.WriteFile(fs, "/cwd/.env",
		[]byte("NEO4J_PASSWORD=cwd\n"), 0644))

	got, err := loadEnvFile(fs, "/elsewhere/custom.env", "/cwd")
	require.NoError(t, err)
	assert.Equal(t, "fromfile", got["NEO4J_PASSWORD"])
}

func TestLoadEnvFile_ExplicitMissingErrors(t *testing.T) {
	fs := afero.NewMemMapFs()
	_, err := loadEnvFile(fs, "/no/such/file", "/cwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/no/such/file")
}

func TestResolveConn_Defaults(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	cmd, cfg := newTestCmd(t)
	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, defaultURI, c.uri)
	assert.Equal(t, defaultUsername, c.username)
	assert.Equal(t, "", c.password)
	assert.Equal(t, defaultDatabase, c.database)
	// resolveConn does not eagerly open the driver — that happens via
	// c.openDriver() once the password has been prompted (when needed).
	assert.Nil(t, c.driver)
}

func TestResolveConn_PrecedenceFlagsBeatEnvBeatsDotenv(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	t.Setenv(envURI, "http://from-env:7474")
	t.Setenv(envUsername, "fromenv")
	t.Setenv(envPassword, "envpw")
	t.Setenv(envDatabase, "envdb")

	// Use a mem FS so the test is hermetic regardless of real credentials or
	// dotenv files on the machine. Write the dotenv at the temp cwd path so
	// the walk-up logic finds it via cfg.Aura.Fs().
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"),
		[]byte(strings.Join([]string{
			"NEO4J_URI=http://from-dotenv:7474",
			"NEO4J_USERNAME=fromdotenv",
			"NEO4J_PASSWORD=dotenv-pw",
			"NEO4J_DATABASE=dotenvdb",
		}, "\n")+"\n"), 0644))
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=http://from-flag:7474",
		"--database=flagdb",
	}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	// uri+database: flag wins outright.
	assert.Equal(t, "http://from-flag:7474", c.uri)
	assert.Equal(t, "flagdb", c.database)
	// username+password: no flag → env wins over .env.
	assert.Equal(t, "fromenv", c.username)
	assert.Equal(t, "envpw", c.password)
}

func TestResolveConn_DotenvWinsWhenNoEnvOrFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")

	// Use a mem FS so the test is hermetic regardless of real credentials on the
	// machine. Write the dotenv at the temp cwd path so the walk-up logic finds
	// it via cfg.Aura.Fs().
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, filepath.Join(tmp, ".env"),
		[]byte("NEO4J_USERNAME=onlydotenv\nNEO4J_PASSWORD=onlydotenvpw\n"), 0644))
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "onlydotenv", c.username)
	assert.Equal(t, "onlydotenvpw", c.password)
}

// withRunStatementSeam swaps the package-level runStatementResponseFn AND
// driverOpener for the duration of the test, so individual tests can inject
// canned responses without booting a real Neo4j server. The driverOpener stub
// returns a no-op driver so deferred Close calls in production code are safe.
// Restored via t.Cleanup. The seam fn receives the readOnly flag so tests can
// assert callers route ExecuteRead vs ExecuteWrite correctly.
func withRunStatementSeam(t *testing.T, fn func(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResponse, error)) {
	t.Helper()
	origFn := runStatementResponseFn
	origOpener := driverOpener
	t.Cleanup(func() {
		runStatementResponseFn = origFn
		driverOpener = origOpener
	})
	runStatementResponseFn = fn
	driverOpener = func(target, username, password, userAgent string) (neo4j.Driver, error) {
		return &noopDriver{}, nil
	}
}

// noopDriver is a stub neo4j.Driver used by tests that route
// statement execution through the runStatementResponseFn seam. The embedded
// nil interface satisfies the wide DriverWithContext interface at the type
// level; only Close is invoked by production code (via defer in run.go and
// schema.go) so we provide a real Close that returns nil. Any other method
// call on this stub indicates the seam was bypassed and should fail loudly
// at runtime — embedding nil panics on call, which is the desired signal.
type noopDriver struct {
	neo4j.Driver
}

func (n *noopDriver) Close(ctx context.Context) error { return nil }

func TestRunStatement_HappyPath(t *testing.T) {
	var gotStatement string
	var gotParams map[string]any
	var gotReadOnly bool
	withRunStatementSeam(t, func(_ context.Context, _ *conn, statement string, params map[string]any, readOnly bool) (*queryResponse, error) {
		gotStatement = statement
		gotParams = params
		gotReadOnly = readOnly
		resp := &queryResponse{}
		resp.Data.Fields = []string{"n"}
		resp.Data.Values = [][]any{{int64(1)}}
		return resp, nil
	})

	c := &conn{
		uri:       "neo4j://example:7687",
		username:  "neo4j",
		password:  "secret",
		database:  "neo4j",
		userAgent: "neo4j-cli/vtest",
	}

	res, err := runStatement(context.Background(), c, "RETURN 1 AS n", map[string]any{"k": 5})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, []string{"n"}, res.Columns)
	assert.Equal(t, [][]any{{int64(1)}}, res.Rows)

	// Statement and params forwarded to the seam unmodified.
	assert.Equal(t, "RETURN 1 AS n", gotStatement)
	assert.Equal(t, map[string]any{"k": 5}, gotParams)
	// runStatement defaults to the read-only path (ExecuteRead).
	assert.True(t, gotReadOnly, "runStatement must route through ExecuteRead by default")
}

func TestRunStatement_ExplainResponseCarriesQueryType(t *testing.T) {
	withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
		resp := &queryResponse{}
		resp.Data.Fields = []string{}
		resp.Data.Values = [][]any{}
		resp.QueryType = neo4j.QueryTypeReadWrite
		resp.Bookmarks = []string{"FB:test"}
		return resp, nil
	})

	c := &conn{uri: "neo4j://example:7687", database: "neo4j"}

	res, err := runStatement(context.Background(), c, "EXPLAIN CREATE (n)", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{}, res.Columns)
	assert.Equal(t, [][]any{}, res.Rows)

	resp, err := runStatementResponse(context.Background(), c, "EXPLAIN CREATE (n)", nil, true)
	require.NoError(t, err)
	assert.Equal(t, neo4j.QueryTypeReadWrite, resp.QueryType)
	assert.Equal(t, []string{"FB:test"}, resp.Bookmarks)
}

func TestResolveConn_UserAgent(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"populated version", "1.2.3", "neo4j-cli/v1.2.3"},
		{"empty falls back to dev", "", "neo4j-cli/vdev"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envURI, "")
			t.Setenv(envUsername, "")
			t.Setenv(envPassword, "")
			t.Setenv(envDatabase, "")
			t.Chdir(t.TempDir())

			fs := afero.NewMemMapFs()
			cfg := clicfg.NewConfig(fs, tc.version, clicfg.QueryScope)
			cmd := NewCmd(cfg)

			c, err := resolveConn(cmd, cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.want, c.userAgent)
		})
	}
}

func TestRunStatementWrite_RoutesThroughExecuteWrite(t *testing.T) {
	var gotReadOnly bool
	withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, readOnly bool) (*queryResponse, error) {
		gotReadOnly = readOnly
		resp := &queryResponse{}
		resp.Data.Fields = []string{}
		resp.Data.Values = [][]any{}
		return resp, nil
	})

	c := &conn{database: "neo4j"}
	_, err := runStatementWrite(context.Background(), c, "CREATE (n)", nil)
	require.NoError(t, err)
	assert.False(t, gotReadOnly, "runStatementWrite must route through ExecuteWrite (readOnly=false)")
}

func TestRunStatement_ServerErrorSurfacesError(t *testing.T) {
	withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
		return nil, &fakeNeo4jError{code: "Neo.ClientError.Statement.SyntaxError", message: "Invalid input"}
	})

	c := &conn{database: "neo4j"}
	_, err := runStatement(context.Background(), c, "BAD CYPHER", nil)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Neo.ClientError.Statement.SyntaxError")
	assert.Contains(t, msg, "Invalid input")
}

// fakeNeo4jError mimics the shape of an upstream driver error wrapping a code
// + message — its String/Error format is used when runStatement wraps the
// driver error and surfaces it to callers.
type fakeNeo4jError struct {
	code    string
	message string
}

func (e *fakeNeo4jError) Error() string {
	return e.code + ": " + e.message
}

// newTestCmdWithCreds returns a query command and config backed by an in-memory
// filesystem that already has credentials.json populated with the supplied JSON.
func newTestCmdWithCreds(t *testing.T, credsJSON string) (*cobra.Command, *clicfg.Config) {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, credsJSON)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)
	cmd := NewCmd(cfg)
	return cmd, cfg
}

// storedCredJSON returns a credentials.json body with one dbms credential
// set as the default.
func storedCredJSON(uri, username, password, dbName string) string {
	return `{"dbms":{"default-credential":"mydb","credentials":[{"name":"mydb","username":"` +
		username + `","password":"` + password + `","database-name":"` + dbName +
		`","uri":"` + uri + `"}]}}`
}

func TestResolveConn_StoredCredential_UsedWhenNoFlagsOrEnv(t *testing.T) {
	// Clear all env vars so the stored credential is the only source.
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB")
	cmd, cfg := newTestCmdWithCreds(t, credsJSON)

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, "http://stored:7474", c.uri)
	assert.Equal(t, "storedUser", c.username)
	assert.Equal(t, "storedPass", c.password)
	assert.Equal(t, "storedDB", c.database)
}

func TestResolveConn_StoredCredential_AllFourFlagsBypassCredential(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB")
	cmd, cfg := newTestCmdWithCreds(t, credsJSON)
	require.NoError(t, cmd.ParseFlags([]string{
		"--uri=http://flag:7474",
		"--username=flagUser",
		"--password=flagPass",
		"--database=flagDB",
	}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, "http://flag:7474", c.uri)
	assert.Equal(t, "flagUser", c.username)
	assert.Equal(t, "flagPass", c.password)
	assert.Equal(t, "flagDB", c.database)
}

func TestResolveConn_StoredCredential_PartialOverrideErrors(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB")
	cmd, cfg := newTestCmdWithCreds(t, credsJSON)
	// Only one of the four params provided — ambiguous partial override.
	require.NoError(t, cmd.ParseFlags([]string{"--uri=http://override:7474"}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--uri/NEO4J_URI")
	assert.Contains(t, err.Error(), "--username/NEO4J_USERNAME")
	assert.Contains(t, err.Error(), "--password/NEO4J_PASSWORD")
	assert.Contains(t, err.Error(), "--database/NEO4J_DATABASE")
}

func TestResolveConn_NoStoredCredential_FallsBackToDefaults(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	// Empty credentials — no stored credential.
	cmd, cfg := newTestCmdWithCreds(t, "{}")

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, defaultURI, c.uri)
	assert.Equal(t, defaultUsername, c.username)
	assert.Equal(t, "", c.password)
	assert.Equal(t, defaultDatabase, c.database)
}

// namedCredJSON returns a credentials.json body with one named credential
// (not necessarily set as the default).
func namedCredJSON(name, uri, username, password, dbName string) string {
	return `{"dbms":{"default-credential":"","credentials":[{"name":"` + name +
		`","username":"` + username + `","password":"` + password +
		`","database-name":"` + dbName + `","uri":"` + uri + `"}]}}`
}

func TestResolveConn_CredentialFlag(t *testing.T) {
	twoCredsJSON := `{"dbms":{"default-credential":"default-cred","credentials":[` +
		`{"name":"default-cred","username":"defaultUser","password":"defaultPass","database-name":"defaultDB","uri":"http://default:7474"},` +
		`{"name":"other-cred","username":"otherUser","password":"otherPass","database-name":"otherDB","uri":"http://other:7474"}` +
		`]}}`

	tests := []struct {
		name            string
		credsJSON       string
		flags           []string
		wantErrContains []string
		wantURI         string
		wantUsername    string
		wantPassword    string
		wantDatabase    string
	}{
		{
			name:         "resolves named credential",
			credsJSON:    namedCredJSON("mydb", "http://named:7474", "namedUser", "namedPass", "namedDB"),
			flags:        []string{"--credential=mydb"},
			wantURI:      "http://named:7474",
			wantUsername: "namedUser",
			wantPassword: "namedPass",
			wantDatabase: "namedDB",
		},
		{
			name:            "conflicts with --username",
			credsJSON:       namedCredJSON("mydb", "http://named:7474", "namedUser", "namedPass", "namedDB"),
			flags:           []string{"--credential=mydb", "--username=other"},
			wantErrContains: []string{"--credential", "--username"},
		},
		{
			name:            "unknown credential errors with helpful message",
			credsJSON:       "{}",
			flags:           []string{"--credential=unknown"},
			wantErrContains: []string{"unknown", "credential dbms list"},
		},
		{
			name:         "no --credential flag uses stored default (existing behaviour unchanged)",
			credsJSON:    storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB"),
			flags:        []string{},
			wantURI:      "http://stored:7474",
			wantUsername: "storedUser",
			wantPassword: "storedPass",
			wantDatabase: "storedDB",
		},
		{
			name:         "overrides stored default credential",
			credsJSON:    twoCredsJSON,
			flags:        []string{"--credential=other-cred"},
			wantURI:      "http://other:7474",
			wantUsername: "otherUser",
			wantPassword: "otherPass",
			wantDatabase: "otherDB",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envURI, "")
			t.Setenv(envUsername, "")
			t.Setenv(envPassword, "")
			t.Setenv(envDatabase, "")
			t.Chdir(t.TempDir())

			cmd, cfg := newTestCmdWithCreds(t, tc.credsJSON)
			require.NoError(t, cmd.ParseFlags(tc.flags))

			c, err := resolveConn(cmd, cfg)

			if len(tc.wantErrContains) > 0 {
				require.Error(t, err)
				for _, s := range tc.wantErrContains {
					assert.Contains(t, err.Error(), s)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantURI, c.uri)
			assert.Equal(t, tc.wantUsername, c.username)
			assert.Equal(t, tc.wantPassword, c.password)
			assert.Equal(t, tc.wantDatabase, c.database)
		})
	}
}
