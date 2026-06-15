// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
)

// TestResolveConn_DesktopActive_Resolves verifies REQ-F-101: `--credential
// desktop` resolves to the single status==started DBMS, with credentials
// flowing through unchanged.
func TestResolveConn_DesktopActive_Resolves(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
		return &dbconn.DesktopMatch{
			Dbms: &desktopclient.DbmsInfo{
				ID:            "running-dbms",
				Name:          "running",
				Status:        "started",
				ConnectionURI: "neo4j://localhost:7690",
			},
			Creds: &desktopclient.Credentials{Username: "neo4j", Password: "running-pw"},
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://localhost:7690", c.URI)
	assert.Equal(t, "neo4j", c.Username)
	assert.Equal(t, "running-pw", c.Password)
	// Database left unset → server resolves the home database (CLI-211).
	assert.Equal(t, "", c.Database)
}

// TestResolveConn_DesktopActive_DatabaseOverride verifies CLI-212 on the
// `desktop` prefix: --database / NEO4J_DATABASE override the (empty)
// desktop-supplied database with flag > env > none precedence.
func TestResolveConn_DesktopActive_DatabaseOverride(t *testing.T) {
	tests := []struct {
		name         string
		flags        []string
		envDB        string
		wantDatabase string
	}{
		{name: "flag overrides", flags: []string{"--database=movies"}, wantDatabase: "movies"},
		{name: "env applies when flag unset", envDB: "envdb", wantDatabase: "envdb"},
		{name: "flag beats env", flags: []string{"--database=movies"}, envDB: "envdb", wantDatabase: "movies"},
		{name: "no override leaves home DB resolution", wantDatabase: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(dbconn.EnvURI, "")
			t.Setenv(dbconn.EnvUsername, "")
			t.Setenv(dbconn.EnvPassword, "")
			t.Setenv(dbconn.EnvDatabase, tc.envDB)
			t.Chdir(t.TempDir())

			restore := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
				return &dbconn.DesktopMatch{
					Dbms: &desktopclient.DbmsInfo{
						ID:            "running-dbms",
						Name:          "running",
						Status:        "started",
						ConnectionURI: "neo4j://localhost:7690",
					},
					Creds: &desktopclient.Credentials{Username: "neo4j", Password: "running-pw"},
				}, nil
			})
			defer restore()

			cmd, cfg := newTestCmdWithCreds(t, "{}")
			require.NoError(t, cmd.ParseFlags(append([]string{"--credential=desktop"}, tc.flags...)))

			c, err := resolveConn(cmd, cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDatabase, c.Database)
		})
	}
}

// TestResolveConn_DesktopActive_NullCreds_TTYPrompts verifies REQ-F-028 on
// the `desktop` prefix: Desktop returned a DBMS but no creds; on a TTY we
// prompt and assemble the conn with the prompted password.
func TestResolveConn_DesktopActive_NullCreds_TTYPrompts(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	origPwReader := dbconn.PasswordReader
	dbconn.StdinIsTTY = func() bool { return true }
	dbconn.PasswordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() {
		dbconn.StdinIsTTY = origIsTTY
		dbconn.PasswordReader = origPwReader
	})

	restore := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
		return &dbconn.DesktopMatch{
			Dbms: &desktopclient.DbmsInfo{
				ID:            "legacy-dbms",
				Name:          "legacy",
				Status:        "started",
				ConnectionURI: "neo4j://localhost:7777",
			},
			Creds: nil,
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://localhost:7777", c.URI)
	assert.Equal(t, dbconn.DefaultUsername, c.Username)
	assert.Equal(t, "prompted-pw", c.Password)
}

// TestResolveConn_DesktopActive_NullCreds_NonTTYFatals verifies REQ-F-028 on
// the `desktop` prefix non-TTY branch: no stored creds, no TTY → fatal with
// the 3-option hint.
func TestResolveConn_DesktopActive_NullCreds_NonTTYFatals(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return false }
	t.Cleanup(func() { dbconn.StdinIsTTY = origIsTTY })

	restore := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
		return &dbconn.DesktopMatch{
			Dbms: &desktopclient.DbmsInfo{
				ID:     "legacy-dbms",
				Name:   "legacy",
				Status: "started",
			},
			Creds: nil,
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
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
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
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
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

	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := dbconn.SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*dbconn.DesktopMatch, error) {
		require.Equal(t, validUUID, raw)
		return &dbconn.DesktopMatch{
			Connection: &desktopclient.Connection{
				ID:            validUUID,
				Name:          "prod-aura",
				ConnectionURI: "neo4j+s://example.databases.neo4j.io",
			},
			Creds: &desktopclient.Credentials{Username: "aura-user", Password: "aura-pw"},
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + validUUID}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j+s://example.databases.neo4j.io", c.URI)
	assert.Equal(t, "aura-user", c.Username)
	assert.Equal(t, "aura-pw", c.Password)
}

// TestResolveConn_DesktopConnection_DatabaseOverride verifies CLI-212 on the
// `desktop-connection:<uuid>` prefix: --database / NEO4J_DATABASE override
// the (empty) connection-supplied database with flag > env > none precedence.
func TestResolveConn_DesktopConnection_DatabaseOverride(t *testing.T) {
	const validUUID = "11111111-2222-3333-4444-555555555555"

	tests := []struct {
		name         string
		flags        []string
		envDB        string
		wantDatabase string
	}{
		{name: "flag overrides", flags: []string{"--database=movies"}, wantDatabase: "movies"},
		{name: "env applies when flag unset", envDB: "envdb", wantDatabase: "envdb"},
		{name: "flag beats env", flags: []string{"--database=movies"}, envDB: "envdb", wantDatabase: "movies"},
		{name: "no override leaves home DB resolution", wantDatabase: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(dbconn.EnvURI, "")
			t.Setenv(dbconn.EnvUsername, "")
			t.Setenv(dbconn.EnvPassword, "")
			t.Setenv(dbconn.EnvDatabase, tc.envDB)
			t.Chdir(t.TempDir())

			restore := dbconn.SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, _ string) (*dbconn.DesktopMatch, error) {
				return &dbconn.DesktopMatch{
					Connection: &desktopclient.Connection{
						ID:            validUUID,
						Name:          "prod-aura",
						ConnectionURI: "neo4j+s://example.databases.neo4j.io",
					},
					Creds: &desktopclient.Credentials{Username: "aura-user", Password: "aura-pw"},
				}, nil
			})
			defer restore()

			cmd, cfg := newTestCmdWithCreds(t, "{}")
			require.NoError(t, cmd.ParseFlags(append([]string{"--credential=desktop-connection:" + validUUID}, tc.flags...)))

			c, err := resolveConn(cmd, cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.wantDatabase, c.Database)
		})
	}
}

// TestResolveConn_DesktopConnection_MalformedUUID_UsageError verifies
// REQ-F-102: a non-UUID value after the prefix surfaces a usage error
// pointing at `desktop list`.
func TestResolveConn_DesktopConnection_MalformedUUID_UsageError(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
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

	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := dbconn.SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*dbconn.DesktopMatch, error) {
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

	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := dbconn.StdinIsTTY
	origPwReader := dbconn.PasswordReader
	dbconn.StdinIsTTY = func() bool { return true }
	dbconn.PasswordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() {
		dbconn.StdinIsTTY = origIsTTY
		dbconn.PasswordReader = origPwReader
	})

	restore := dbconn.SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, _ string) (*dbconn.DesktopMatch, error) {
		return &dbconn.DesktopMatch{
			Connection: &desktopclient.Connection{
				ID:            validUUID,
				Name:          "no-creds-conn",
				ConnectionURI: "neo4j+s://example.databases.neo4j.io",
			},
			Creds: nil,
		}, nil
	})
	defer restore()

	cmd, cfg := newTestCmdWithCreds(t, "{}")
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + validUUID}))

	c, err := resolveConn(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, "neo4j+s://example.databases.neo4j.io", c.URI)
	assert.Equal(t, dbconn.DefaultUsername, c.Username)
	assert.Equal(t, "prompted-pw", c.Password)
}

// TestResolveConn_UnprefixedCredName_NoDesktopFallthrough verifies REQ-F-103
// (the headline regression gate): an unprefixed `--credential <name>` value
// never instantiates the Desktop client. The seams panic the test if invoked.
func TestResolveConn_UnprefixedCredName_NoDesktopFallthrough(t *testing.T) {
	t.Setenv(dbconn.EnvURI, "")
	t.Setenv(dbconn.EnvUsername, "")
	t.Setenv(dbconn.EnvPassword, "")
	t.Setenv(dbconn.EnvDatabase, "")
	t.Chdir(t.TempDir())

	restoreActive := dbconn.SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*dbconn.DesktopMatch, error) {
		t.Fatal("REQ-F-103: unprefixed --credential must NOT call the desktop active-DBMS resolver")
		return nil, nil
	})
	defer restoreActive()

	restoreConn := dbconn.SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, _ string) (*dbconn.DesktopMatch, error) {
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

// resolveDesktopActiveDbmsCredentialFromList is a tiny test helper that
// applies the same filtering logic the production resolver uses, against an
// in-memory list of DbmsInfo (no httptest required). Used by tests that need
// to assert the multi-running / no-running error shapes without re-spinning
// the relate mock server.
func resolveDesktopActiveDbmsCredentialFromList(list []desktopclient.DbmsInfo) (*dbconn.DesktopMatch, error) {
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
		return &dbconn.DesktopMatch{Dbms: &dbms}, nil
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
func resolveDesktopConnectionCredentialFromList(raw string, list []desktopclient.Connection) (*dbconn.DesktopMatch, error) {
	for i := range list {
		if list[i].ID == raw {
			return &dbconn.DesktopMatch{Connection: &list[i]}, nil
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
