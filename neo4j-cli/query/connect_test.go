// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	cmd, cfg := newTestCmd(t)
	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, defaultURI, c.uri)
	assert.Equal(t, defaultUsername, c.username)
	assert.Equal(t, "", c.password)
	assert.Equal(t, defaultDatabase, c.database)
	assert.False(t, c.insecure)
	assert.NotNil(t, c.doer)
}

func TestResolveConn_PrecedenceFlagsBeatEnvBeatsDotenv(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	t.Setenv(envURI, "http://from-env:7474")
	t.Setenv(envUsername, "fromenv")
	t.Setenv(envPassword, "envpw")
	t.Setenv(envDatabase, "envdb")
	t.Setenv(envInsecure, "")

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
	t.Setenv(envInsecure, "")

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

func TestResolveConn_InsecureFromEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(envInsecure, "true")

	cmd, cfg := newTestCmd(t)
	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.True(t, c.insecure)
}

func TestResolveConn_InsecureFlagOverridesEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(envInsecure, "true")

	cmd, cfg := newTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--insecure=false"}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.False(t, c.insecure)
}

func TestRunStatement_HappyPath(t *testing.T) {
	var gotPath, gotAuth, gotMethod, gotCT, gotUA string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotUA = r.Header.Get("User-Agent")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"fields":["n"],"values":[[1]]}}`))
	}))
	defer srv.Close()

	c := &conn{
		uri:       srv.URL,
		username:  "neo4j",
		password:  "secret",
		database:  "neo4j",
		userAgent: "neo4j-cli/vtest",
		doer:      srv.Client(),
	}

	res, err := runStatement(context.Background(), c, "RETURN 1 AS n", map[string]any{"k": 5})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, []string{"n"}, res.Columns)
	assert.Equal(t, [][]any{{float64(1)}}, res.Rows)

	// Server saw the right URL, method, headers, body shape.
	assert.Equal(t, "/db/neo4j/query/v2", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotCT)
	assert.Equal(t, "neo4j-cli/vtest", gotUA)

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("neo4j:secret"))
	assert.Equal(t, wantAuth, gotAuth)

	assert.Equal(t, "RETURN 1 AS n", gotBody["statement"])
	require.Contains(t, gotBody, "parameters")
	assert.Equal(t, map[string]any{"k": float64(5)}, gotBody["parameters"])
	// txMetadata is intentionally NOT sent — see runStatement doc comment for
	// the Neo4j 2026.04+ minimum-version constraint that defers it.
	assert.NotContains(t, gotBody, "txMetadata")
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
			t.Setenv(envInsecure, "")
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

func TestRunStatement_NoParamsOmitsField(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"fields":[],"values":[]}}`))
	}))
	defer srv.Close()

	c := &conn{uri: srv.URL, username: "u", password: "p", database: "neo4j", userAgent: "neo4j-cli/vtest", doer: srv.Client()}
	_, err := runStatement(context.Background(), c, "RETURN 1", nil)
	require.NoError(t, err)
	_, hasParams := gotBody["parameters"]
	assert.False(t, hasParams, "omit parameters when nil")
}

func TestRunStatement_ServerErrorSurfacesCodeAndMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"Neo.ClientError.Statement.SyntaxError","message":"Invalid input"}]}`))
	}))
	defer srv.Close()

	c := &conn{uri: srv.URL, username: "u", password: "p", database: "neo4j", userAgent: "neo4j-cli/vtest", doer: srv.Client()}
	_, err := runStatement(context.Background(), c, "BAD CYPHER", nil)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Neo.ClientError.Statement.SyntaxError")
	assert.Contains(t, msg, "Invalid input")
}

func TestRunStatement_Non2xxWithoutErrorsField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := &conn{uri: srv.URL, username: "u", password: "p", database: "neo4j", userAgent: "neo4j-cli/vtest", doer: srv.Client()}
	_, err := runStatement(context.Background(), c, "RETURN 1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestRunStatement_DatabaseInPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":{"fields":[],"values":[]}}`))
	}))
	defer srv.Close()

	c := &conn{uri: srv.URL, username: "u", password: "p", database: "movies", userAgent: "neo4j-cli/vtest", doer: srv.Client()}
	_, err := runStatement(context.Background(), c, "RETURN 1", nil)
	require.NoError(t, err)
	assert.Equal(t, "/db/movies/query/v2", gotPath)
}

func TestNewHTTPClient_InsecureFlipsSkipVerify(t *testing.T) {
	secure := newHTTPClient(false)
	insecure := newHTTPClient(true)

	// Default client has nil Transport.
	assert.Nil(t, secure.Transport)

	tr, ok := insecure.Transport.(*http.Transport)
	require.True(t, ok, "insecure client must use *http.Transport")
	require.NotNil(t, tr.TLSClientConfig)
	assert.True(t, tr.TLSClientConfig.InsecureSkipVerify)
}

func TestRunStatement_TLSWithoutInsecureFails(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"fields":[],"values":[]}}`))
	}))
	defer srv.Close()

	// Use the secure default client (no skip-verify); httptest's self-signed
	// cert should fail verification.
	c := &conn{uri: srv.URL, username: "u", password: "p", database: "neo4j", userAgent: "neo4j-cli/vtest", doer: newHTTPClient(false)}
	_, err := runStatement(context.Background(), c, "RETURN 1", nil)
	require.Error(t, err)
	// The exact error wording depends on Go version but always references TLS/cert.
	low := strings.ToLower(err.Error())
	assert.True(t,
		strings.Contains(low, "certificate") ||
			strings.Contains(low, "tls") ||
			strings.Contains(low, "x509"),
		"expected TLS-related error, got: %s", err.Error())
}

func TestRunStatement_TLSWithInsecureSucceeds(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"fields":["n"],"values":[[1]]}}`))
	}))
	defer srv.Close()

	c := &conn{uri: srv.URL, username: "u", password: "p", database: "neo4j", userAgent: "neo4j-cli/vtest", doer: newHTTPClient(true)}
	res, err := runStatement(context.Background(), c, "RETURN 1", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"n"}, res.Columns)
}

func TestParseBool(t *testing.T) {
	tests := []struct {
		in         string
		want       bool
		recognised bool
	}{
		{"", false, false},
		{"true", true, true},
		{"false", false, true},
		{"1", true, true},
		{"0", false, true},
		{"yes", true, true},
		{"NO", false, true},
		{"On", true, true},
		{"off", false, true},
		{"banana", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			b, ok := parseBool(tc.in)
			assert.Equal(t, tc.want, b)
			assert.Equal(t, tc.recognised, ok)
		})
	}
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

// storedCredJSON returns a credentials.json body with one database credential
// set as the default.
func storedCredJSON(uri, username, password, dbName string, insecure bool) string {
	insecureStr := "false"
	if insecure {
		insecureStr = "true"
	}
	return `{"database":{"default-credential":"mydb","credentials":[{"name":"mydb","username":"` +
		username + `","password":"` + password + `","database-name":"` + dbName +
		`","uri":"` + uri + `","insecure":` + insecureStr + `}]}}`
}

func TestResolveConn_StoredCredential_UsedWhenNoFlagsOrEnv(t *testing.T) {
	// Clear all env vars so the stored credential is the only source.
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB", false)
	cmd, cfg := newTestCmdWithCreds(t, credsJSON)

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, "http://stored:7474", c.uri)
	assert.Equal(t, "storedUser", c.username)
	assert.Equal(t, "storedPass", c.password)
	assert.Equal(t, "storedDB", c.database)
	assert.False(t, c.insecure)
}

func TestResolveConn_StoredCredential_Insecure_AppliedWithoutFlag(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "u", "p", "neo4j", true)
	cmd, cfg := newTestCmdWithCreds(t, credsJSON)

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.True(t, c.insecure, "stored credential's insecure:true must be applied when --insecure flag is not set")
}

func TestResolveConn_StoredCredential_InsecureFlagOverridesCredential(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	// Stored credential has insecure=true, but --insecure=false is passed explicitly.
	credsJSON := storedCredJSON("http://stored:7474", "u", "p", "neo4j", true)
	cmd, cfg := newTestCmdWithCreds(t, credsJSON)
	require.NoError(t, cmd.ParseFlags([]string{"--insecure=false"}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.False(t, c.insecure, "--insecure=false flag must override stored credential's insecure:true")
}

func TestResolveConn_StoredCredential_AllFourFlagsBypassCredential(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB", false)
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
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	credsJSON := storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB", false)
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
	t.Setenv(envInsecure, "")
	t.Chdir(t.TempDir())

	// Empty credentials — no stored credential.
	cmd, cfg := newTestCmdWithCreds(t, "{}")

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)

	assert.Equal(t, defaultURI, c.uri)
	assert.Equal(t, defaultUsername, c.username)
	assert.Equal(t, "", c.password)
	assert.Equal(t, defaultDatabase, c.database)
	assert.False(t, c.insecure)
}

// namedCredJSON returns a credentials.json body with one named credential
// (not necessarily set as the default).
func namedCredJSON(name, uri, username, password, dbName string, insecure bool) string {
	insecureStr := "false"
	if insecure {
		insecureStr = "true"
	}
	return `{"database":{"default-credential":"","credentials":[{"name":"` + name +
		`","username":"` + username + `","password":"` + password +
		`","database-name":"` + dbName + `","uri":"` + uri +
		`","insecure":` + insecureStr + `}]}}`
}

func TestResolveConn_CredentialFlag(t *testing.T) {
	twoCredsJSON := `{"database":{"default-credential":"default-cred","credentials":[` +
		`{"name":"default-cred","username":"defaultUser","password":"defaultPass","database-name":"defaultDB","uri":"http://default:7474","insecure":false},` +
		`{"name":"other-cred","username":"otherUser","password":"otherPass","database-name":"otherDB","uri":"http://other:7474","insecure":false}` +
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
		wantInsecure    bool
	}{
		{
			name:         "resolves named credential",
			credsJSON:    namedCredJSON("mydb", "http://named:7474", "namedUser", "namedPass", "namedDB", false),
			flags:        []string{"--credential=mydb"},
			wantURI:      "http://named:7474",
			wantUsername: "namedUser",
			wantPassword: "namedPass",
			wantDatabase: "namedDB",
			wantInsecure: false,
		},
		{
			name:            "conflicts with --username",
			credsJSON:       namedCredJSON("mydb", "http://named:7474", "namedUser", "namedPass", "namedDB", false),
			flags:           []string{"--credential=mydb", "--username=other"},
			wantErrContains: []string{"--credential", "--username"},
		},
		{
			name:            "unknown credential errors with helpful message",
			credsJSON:       "{}",
			flags:           []string{"--credential=unknown"},
			wantErrContains: []string{"unknown", "credential database list"},
		},
		{
			name:         "--insecure=false overrides credential's insecure:true",
			credsJSON:    namedCredJSON("mydb", "http://named:7474", "u", "p", "neo4j", true),
			flags:        []string{"--credential=mydb", "--insecure=false"},
			wantURI:      "http://named:7474",
			wantUsername: "u",
			wantPassword: "p",
			wantDatabase: "neo4j",
			wantInsecure: false,
		},
		{
			name:         "credential's insecure:true applied when --insecure not set",
			credsJSON:    namedCredJSON("mydb", "http://named:7474", "u", "p", "neo4j", true),
			flags:        []string{"--credential=mydb"},
			wantURI:      "http://named:7474",
			wantUsername: "u",
			wantPassword: "p",
			wantDatabase: "neo4j",
			wantInsecure: true,
		},
		{
			name:         "no --credential flag uses stored default (existing behaviour unchanged)",
			credsJSON:    storedCredJSON("http://stored:7474", "storedUser", "storedPass", "storedDB", false),
			flags:        []string{},
			wantURI:      "http://stored:7474",
			wantUsername: "storedUser",
			wantPassword: "storedPass",
			wantDatabase: "storedDB",
			wantInsecure: false,
		},
		{
			name:         "overrides stored default credential",
			credsJSON:    twoCredsJSON,
			flags:        []string{"--credential=other-cred"},
			wantURI:      "http://other:7474",
			wantUsername: "otherUser",
			wantPassword: "otherPass",
			wantDatabase: "otherDB",
			wantInsecure: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envURI, "")
			t.Setenv(envUsername, "")
			t.Setenv(envPassword, "")
			t.Setenv(envDatabase, "")
			t.Setenv(envInsecure, "")
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
			assert.Equal(t, tc.wantInsecure, c.insecure)
		})
	}
}
