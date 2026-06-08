// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
)

// newDesktopFallthroughFs returns an in-memory afero.Fs that satisfies the
// production fall-through preconditions: relate env JSONs are written below
// EnvConfigDir() (per-OS; see desktopclient/envconfig.go) so LoadEnvs returns
// one active env, and `relate.secret.key` is seeded inside that env's data
// dir so LoadSalt succeeds.
//
// The helper installs a userConfigDir override pointing at `/cfg` and a
// goosFn override pinning the OS to linux so the env-dir layout is stable
// regardless of the host runner. Both overrides are restored via t.Cleanup.
//
// `host` should be a httptest server origin so the salt-keyed JWT verifies
// against the same composite key the test server uses.
func newDesktopFallthroughFs(t *testing.T, host, salt string) (afero.Fs, string) {
	t.Helper()

	// Pin the OS sentinel + userConfigDir so EnvConfigDir() resolves to a
	// deterministic path on every CI runner.
	t.Cleanup(desktopclient.SetGOOSFnForTest(func() string { return "linux" }))
	t.Cleanup(desktopclient.SetUserConfigDirFnForTest(func() (string, error) {
		return "/cfg", nil
	}))

	envsDir, err := desktopclient.EnvConfigDir()
	require.NoError(t, err)

	fs := afero.NewMemMapFs()
	const dataRoot = "/data"
	require.NoError(t, fs.MkdirAll(envsDir, 0o755))

	envJSON, err := json.Marshal(map[string]any{
		"name":           "test-env",
		"id":             "env-1",
		"active":         true,
		"type":           "LOCAL",
		"relateDataPath": dataRoot,
		"httpOrigin":     host,
	})
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, envsDir+"/test-env.json", envJSON, 0o644))

	require.NoError(t, fs.MkdirAll(dataRoot, 0o755))
	require.NoError(t, afero.WriteFile(fs, dataRoot+"/relate.secret.key", []byte(salt), 0o600))

	return fs, dataRoot
}

// TestResolveConn_DesktopActive_Resolves verifies REQ-F-101: `--credential
// desktop` resolves to the single status==started DBMS, with credentials
// flowing through unchanged.
func TestResolveConn_DesktopActive_Resolves(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*desktopMatch, error) {
		return &desktopMatch{
			dbms: &desktopclient.DbmsInfo{
				ID:            "running-dbms",
				Name:          "running",
				Status:        "started",
				ConnectionURI: "neo4j://localhost:7690",
			},
			creds: &desktopclient.Credentials{Username: "neo4j", Password: "running-pw"},
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://localhost:7690", c.uri)
	assert.Equal(t, "neo4j", c.username)
	assert.Equal(t, "running-pw", c.password)
	// Database left unset → server resolves the home database (CLI-211).
	assert.Equal(t, "", c.database)
}

// TestResolveConn_DesktopActive_NullCreds_TTYPrompts verifies REQ-F-028 on
// the `desktop` prefix: Desktop returned a DBMS but no creds; on a TTY we
// prompt and assemble the conn with the prompted password.
func TestResolveConn_DesktopActive_NullCreds_TTYPrompts(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := stdinIsTTY
	origPwReader := passwordReader
	stdinIsTTY = func() bool { return true }
	passwordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() {
		stdinIsTTY = origIsTTY
		passwordReader = origPwReader
	})

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*desktopMatch, error) {
		return &desktopMatch{
			dbms: &desktopclient.DbmsInfo{
				ID:            "legacy-dbms",
				Name:          "legacy",
				Status:        "started",
				ConnectionURI: "neo4j://localhost:7777",
			},
			creds: nil,
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://localhost:7777", c.uri)
	assert.Equal(t, defaultUsername, c.username)
	assert.Equal(t, "prompted-pw", c.password)
}

// TestResolveConn_DesktopActive_NullCreds_NonTTYFatals verifies REQ-F-028 on
// the `desktop` prefix non-TTY branch: no stored creds, no TTY → fatal with
// the 3-option hint.
func TestResolveConn_DesktopActive_NullCreds_NonTTYFatals(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = origIsTTY })

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*desktopMatch, error) {
		return &desktopMatch{
			dbms: &desktopclient.DbmsInfo{
				ID:     "legacy-dbms",
				Name:   "legacy",
				Status: "started",
			},
			creds: nil,
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy")
	assert.Contains(t, err.Error(), "legacy-dbms")
	assert.Contains(t, err.Error(), "--password")
	assert.Contains(t, err.Error(), "credential dbms add")
	assert.Contains(t, err.Error(), "Reset password")
}

// TestResolveConn_DesktopActive_NoneRunning_Fatals verifies REQ-F-101 zero
// matches: no DBMS in started state → fatal pointing at `desktop dbms start`.
func TestResolveConn_DesktopActive_NoneRunning_Fatals(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*desktopMatch, error) {
		return resolveDesktopActiveDbmsCredentialFromList([]desktopclient.DbmsInfo{
			{ID: "stopped-1", Name: "alpha", Status: "stopped"},
			{ID: "stopped-2", Name: "beta", Status: "stopped"},
		})
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No running DBMS")
	assert.Contains(t, err.Error(), "neo4j-cli desktop dbms start")
}

// TestResolveConn_DesktopActive_MultipleRunning_Fatals verifies REQ-F-101's
// defensive >1 branch: enumerate ids and point at `desktop-connection:<id>`.
func TestResolveConn_DesktopActive_MultipleRunning_Fatals(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*desktopMatch, error) {
		return resolveDesktopActiveDbmsCredentialFromList([]desktopclient.DbmsInfo{
			{ID: "running-1", Name: "alpha", Status: "started"},
			{ID: "running-2", Name: "beta", Status: "started"},
		})
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Multiple running DBMSes")
	assert.Contains(t, err.Error(), "running-1")
	assert.Contains(t, err.Error(), "running-2")
	assert.Contains(t, err.Error(), "desktop-connection:")
}

// TestResolveConn_DesktopConnection_Resolves verifies REQ-F-102: a valid
// UUID-shaped `desktop-connection:<uuid>` value resolves the matching
// saved connection via the seam.
func TestResolveConn_DesktopConnection_Resolves(t *testing.T) {
	const validUUID = "11111111-2222-3333-4444-555555555555"

	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*desktopMatch, error) {
		require.Equal(t, validUUID, raw)
		return &desktopMatch{
			connection: &desktopclient.Connection{
				ID:            validUUID,
				Name:          "prod-aura",
				ConnectionURI: "neo4j+s://example.databases.neo4j.io",
			},
			creds: &desktopclient.Credentials{Username: "aura-user", Password: "aura-pw"},
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + validUUID}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j+s://example.databases.neo4j.io", c.uri)
	assert.Equal(t, "aura-user", c.username)
	assert.Equal(t, "aura-pw", c.password)
}

// TestResolveConn_DesktopConnection_MalformedUUID_UsageError verifies
// REQ-F-102: a non-UUID value after the prefix surfaces a usage error
// pointing at `desktop list`.
func TestResolveConn_DesktopConnection_MalformedUUID_UsageError(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:not-a-uuid"}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-uuid")
	assert.Contains(t, err.Error(), "UUID")
	assert.Contains(t, err.Error(), "neo4j-cli desktop list")
}

// TestResolveConn_DesktopConnection_UnknownUUID_Fatals verifies REQ-F-102:
// a well-formed but unknown UUID surfaces a fatal "no connection with id"
// hint pointing at `desktop list`.
func TestResolveConn_DesktopConnection_UnknownUUID_Fatals(t *testing.T) {
	const unknownUUID = "99999999-2222-3333-4444-555555555555"

	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*desktopMatch, error) {
		require.Equal(t, unknownUUID, raw)
		// Mirror the production error shape — driving it through the real
		// function here would require an httptest server; the seam suffices.
		return resolveDesktopConnectionCredentialFromList(unknownUUID, nil)
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + unknownUUID}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no connection with id "+unknownUUID)
	assert.Contains(t, err.Error(), "neo4j-cli desktop list")
}

// TestResolveConn_DesktopConnection_NullCreds_TTYPrompts verifies REQ-F-028
// on the connection prefix: matched connection but no stored creds; on a
// TTY prompt and assemble the conn.
func TestResolveConn_DesktopConnection_NullCreds_TTYPrompts(t *testing.T) {
	const validUUID = "22222222-3333-4444-5555-666666666666"

	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := stdinIsTTY
	origPwReader := passwordReader
	stdinIsTTY = func() bool { return true }
	passwordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() {
		stdinIsTTY = origIsTTY
		passwordReader = origPwReader
	})

	restore := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, _ string) (*desktopMatch, error) {
		return &desktopMatch{
			connection: &desktopclient.Connection{
				ID:            validUUID,
				Name:          "no-creds-conn",
				ConnectionURI: "neo4j+s://example.databases.neo4j.io",
			},
			creds: nil,
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + validUUID}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j+s://example.databases.neo4j.io", c.uri)
	assert.Equal(t, defaultUsername, c.username)
	assert.Equal(t, "prompted-pw", c.password)
}

// TestResolveConn_UnprefixedCredName_NoDesktopFallthrough verifies REQ-F-103
// (the headline regression gate): an unprefixed `--credential <name>` value
// never instantiates the Desktop client. The seams panic the test if invoked.
func TestResolveConn_UnprefixedCredName_NoDesktopFallthrough(t *testing.T) {
	t.Setenv(envURI, "")
	t.Setenv(envUsername, "")
	t.Setenv(envPassword, "")
	t.Setenv(envDatabase, "")
	t.Chdir(t.TempDir())

	restoreActive := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*desktopMatch, error) {
		t.Fatal("REQ-F-103: unprefixed --credential must NOT call the desktop active-DBMS resolver")
		return nil, nil
	})
	defer restoreActive()

	restoreConn := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, _ string) (*desktopMatch, error) {
		t.Fatal("REQ-F-103: unprefixed --credential must NOT call the desktop connection resolver")
		return nil, nil
	})
	defer restoreConn()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=some-random-name"}))

	_, err := resolveConn(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some-random-name")
	assert.Contains(t, err.Error(), "credential dbms add")
	// Hint mentions the two Desktop prefix forms so the user discovers them.
	assert.Contains(t, err.Error(), "--credential desktop")
	assert.Contains(t, err.Error(), "desktop-connection:")
}

// TestResolveDesktopActiveDbmsCredential_RealClient exercises the production
// `resolveDesktopActiveDbmsCredential` path end-to-end: memfs salt + env JSON,
// pinned desktopclient seams, an httptest.NewServer playing the role of the
// relate API, and an HTTP-do override that rewrites every URL to the test
// server. Asserts that filtering by status=="started" against `/dbmss/info`
// (NOT plain `/dbmss`, which omits status) picks the single live DBMS and
// that the resolved creds round-trip. Regression gate for task-009.
func TestResolveDesktopActiveDbmsCredential_RealClient(t *testing.T) {
	const salt = "deadbeef-1111-2222-3333-444444444444"
	const clientID = "11111111-2222-3333-4444-555555555555"

	t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))

	mux := http.NewServeMux()
	mux.HandleFunc("/fastify/api-docs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// `/dbmss` returns the lightweight shape WITHOUT status — the resolver
	// must NOT hit this endpoint. If it does, status is empty and the filter
	// matches zero DBMSes, reproducing the task-009 bug.
	mux.HandleFunc("/fastify/api/dbmss", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"dbms-1","name":"alpha","connectionUri":"neo4j://localhost:7770"},
			{"id":"dbms-2","name":"beta","connectionUri":"neo4j://localhost:7771"}
		]`))
	})
	mux.HandleFunc("/fastify/api/dbmss/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"dbms-1","name":"alpha","status":"stopped","connectionUri":"neo4j://localhost:7770"},
			{"id":"dbms-2","name":"beta","status":"started","connectionUri":"neo4j://localhost:7771"}
		]`))
	})
	mux.HandleFunc("/fastify/api/credentials/dbms:dbms-2", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"username":"neo4j","password":"beta-pw"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rewriteToSrv := func(req *http.Request) (*http.Response, error) {
		newURL := srv.URL + req.URL.Path
		if req.URL.RawQuery != "" {
			newURL += "?" + req.URL.RawQuery
		}
		newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		newReq.Header = req.Header
		return srv.Client().Do(newReq)
	}
	t.Cleanup(desktopclient.SetHTTPClientFnForTest(func() *http.Client {
		return &http.Client{Transport: roundTripperFunc(rewriteToSrv)}
	}))
	t.Cleanup(desktopclient.SetHTTPDoFnForTest(rewriteToSrv))
	// Discover(ctx, 0) runs the mDNS/dns-sd tiers before the port scan; stub
	// them to miss so the test falls through to the httptest-backed scan and
	// stays hermetic (no real multicast / dns-sd shell-out on darwin CI).
	t.Cleanup(desktopclient.SetMDNSBrowseFnForTest(func(context.Context) (int, bool) { return 0, false }))
	t.Cleanup(desktopclient.SetDNSSDLookupFnForTest(func(context.Context) (int, bool) { return 0, false }))

	fs, _ := newDesktopFallthroughFs(t, srv.URL, salt)

	match, err := resolveDesktopActiveDbmsCredential(context.Background(), fs)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, match.dbms)
	assert.Equal(t, "dbms-2", match.dbms.ID)
	require.NotNil(t, match.creds)
	assert.Equal(t, "neo4j", match.creds.Username)
	assert.Equal(t, "beta-pw", match.creds.Password)
}

// TestResolveDesktopConnectionCredential_RealClient exercises the production
// `resolveDesktopConnectionCredential` path end-to-end via httptest.
func TestResolveDesktopConnectionCredential_RealClient(t *testing.T) {
	const salt = "cafebabe-1111-2222-3333-444444444444"
	const clientID = "aaaaaaaa-2222-3333-4444-555555555555"
	const connUUID = "bbbbbbbb-2222-3333-4444-555555555555"

	t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))

	mux := http.NewServeMux()
	mux.HandleFunc("/fastify/api-docs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/fastify/api/connections", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"` + connUUID + `","name":"prod","connectionUri":"neo4j+s://prod.example:7687"}
		]`))
	})
	mux.HandleFunc("/fastify/api/credentials/connection:"+connUUID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"username":"connuser","password":"connpw"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rewriteToSrv := func(req *http.Request) (*http.Response, error) {
		newURL := srv.URL + req.URL.Path
		if req.URL.RawQuery != "" {
			newURL += "?" + req.URL.RawQuery
		}
		newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		newReq.Header = req.Header
		return srv.Client().Do(newReq)
	}
	t.Cleanup(desktopclient.SetHTTPClientFnForTest(func() *http.Client {
		return &http.Client{Transport: roundTripperFunc(rewriteToSrv)}
	}))
	t.Cleanup(desktopclient.SetHTTPDoFnForTest(rewriteToSrv))
	// Discover(ctx, 0) runs the mDNS/dns-sd tiers before the port scan; stub
	// them to miss so the test falls through to the httptest-backed scan and
	// stays hermetic (no real multicast / dns-sd shell-out on darwin CI).
	t.Cleanup(desktopclient.SetMDNSBrowseFnForTest(func(context.Context) (int, bool) { return 0, false }))
	t.Cleanup(desktopclient.SetDNSSDLookupFnForTest(func(context.Context) (int, bool) { return 0, false }))

	fs, _ := newDesktopFallthroughFs(t, srv.URL, salt)

	match, err := resolveDesktopConnectionCredential(context.Background(), fs, connUUID)
	require.NoError(t, err)
	require.NotNil(t, match)
	require.NotNil(t, match.connection)
	assert.Equal(t, connUUID, match.connection.ID)
	require.NotNil(t, match.creds)
	assert.Equal(t, "connuser", match.creds.Username)
	assert.Equal(t, "connpw", match.creds.Password)
}

// resolveDesktopActiveDbmsCredentialFromList is a tiny test helper that
// applies the same filtering logic the production resolver uses, against an
// in-memory list of DbmsInfo (no httptest required). Used by tests that need
// to assert the multi-running / no-running error shapes without re-spinning
// the relate mock server.
func resolveDesktopActiveDbmsCredentialFromList(list []desktopclient.DbmsInfo) (*desktopMatch, error) {
	running := make([]desktopclient.DbmsInfo, 0, len(list))
	for i := range list {
		if list[i].Status == "started" {
			running = append(running, list[i])
		}
	}
	switch len(running) {
	case 0:
		return nil, errNoRunningDBMS()
	case 1:
		dbms := running[0]
		return &desktopMatch{dbms: &dbms}, nil
	default:
		ids := make([]string, 0, len(running))
		for _, d := range running {
			ids = append(ids, d.ID)
		}
		return nil, errMultipleRunningDBMS(ids)
	}
}

// resolveDesktopConnectionCredentialFromList mirrors the production resolver's
// list-and-match step against an in-memory list. Used by tests that need the
// "unknown UUID" error shape without standing up an httptest server.
func resolveDesktopConnectionCredentialFromList(raw string, list []desktopclient.Connection) (*desktopMatch, error) {
	for i := range list {
		if list[i].ID == raw {
			return &desktopMatch{connection: &list[i]}, nil
		}
	}
	return nil, errUnknownConnectionUUID(raw)
}

// errNoRunningDBMS / errMultipleRunningDBMS / errUnknownConnectionUUID
// return errors with text that mirrors the production resolver verbatim so
// tests assert against the same strings.
func errNoRunningDBMS() error {
	return assertSameMessageNoRunningDBMS
}

func errMultipleRunningDBMS(ids []string) error {
	// Build the same text the resolver uses.
	return &errString{s: "Multiple running DBMSes reported by Neo4j Desktop 2 (" + joinIDs(ids) + "). " +
		"Stop all but one, or pick a saved connection with --credential desktop-connection:<id>."}
}

func errUnknownConnectionUUID(raw string) error {
	return &errString{s: "Neo4j Desktop 2 has no connection with id " + raw + ". " +
		"Run 'neo4j-cli desktop list' to see saved connections."}
}

// joinIDs is a tiny strings.Join shim so the file doesn't pull in strings
// just for the error helper.
func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}

type errString struct{ s string }

func (e *errString) Error() string { return e.s }

var assertSameMessageNoRunningDBMS = &errString{
	s: "No running DBMS in Neo4j Desktop 2. Start one with 'neo4j-cli desktop dbms start <id>'.",
}

// roundTripperFunc adapts a plain func to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
