// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
)

// TestResolveConn_DesktopActive_Resolves verifies `--credential desktop`
// resolves to the running Desktop DBMS and passes through credentials.
func TestResolveConn_DesktopActive_Resolves(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*DesktopMatch, error) {
		return &DesktopMatch{
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

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://localhost:7690", conn.URI)
	assert.Equal(t, "neo4j", conn.Username)
	assert.Equal(t, "running-pw", conn.Password)
}

// TestResolveConn_DesktopActive_NullCreds_TTYPrompts verifies that Desktop
// returning a DBMS with no creds on a TTY prompts for the password.
func TestResolveConn_DesktopActive_NullCreds_TTYPrompts(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := StdinIsTTY
	origPwReader := PasswordReader
	StdinIsTTY = func() bool { return true }
	PasswordReader = func() (string, error) { return "prompted-pw", nil }
	t.Cleanup(func() {
		StdinIsTTY = origIsTTY
		PasswordReader = origPwReader
	})

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*DesktopMatch, error) {
		return &DesktopMatch{
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

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j://localhost:7777", conn.URI)
	assert.Equal(t, DefaultUsername, conn.Username)
	assert.Equal(t, "prompted-pw", conn.Password)
}

// TestResolveConn_DesktopActive_NullCreds_NonTTYFatals verifies that Desktop
// returning a DBMS with no creds on a non-TTY returns a fatal with the 3-option
// hint.
func TestResolveConn_DesktopActive_NullCreds_NonTTYFatals(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	origIsTTY := StdinIsTTY
	StdinIsTTY = func() bool { return false }
	t.Cleanup(func() { StdinIsTTY = origIsTTY })

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*DesktopMatch, error) {
		return &DesktopMatch{
			Dbms: &desktopclient.DbmsInfo{
				ID:     "legacy-dbms",
				Name:   "legacy",
				Status: "started",
			},
			Creds: nil,
		}, nil
	})
	defer restore()

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy")
	assert.Contains(t, err.Error(), "legacy-dbms")
	assert.Contains(t, err.Error(), "--password")
	assert.Contains(t, err.Error(), "credential dbms add")
	assert.Contains(t, err.Error(), "Reset password")
}

// TestResolveConn_DesktopActive_Unreachable verifies that a Desktop resolver
// error propagates.
func TestResolveConn_DesktopActive_Unreachable(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*DesktopMatch, error) {
		return nil, desktopclient.UnreachableError()
	})
	defer restore()

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop"}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
}

// TestResolveConn_DesktopConnection_Resolves verifies that a valid UUID in
// `--credential desktop-connection:<uuid>` resolves the matching connection.
func TestResolveConn_DesktopConnection_Resolves(t *testing.T) {
	const validUUID = "11111111-2222-3333-4444-555555555555"

	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*DesktopMatch, error) {
		require.Equal(t, validUUID, raw)
		return &DesktopMatch{
			Connection: &desktopclient.Connection{
				ID:            validUUID,
				Name:          "prod-aura",
				ConnectionURI: "neo4j+s://example.databases.neo4j.io",
			},
			Creds: &desktopclient.Credentials{Username: "aura-user", Password: "aura-pw"},
		}, nil
	})
	defer restore()

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + validUUID}))

	conn, err := ResolveConn(cmd, cfg, true)
	require.NoError(t, err)
	assert.Equal(t, "neo4j+s://example.databases.neo4j.io", conn.URI)
	assert.Equal(t, "aura-user", conn.Username)
	assert.Equal(t, "aura-pw", conn.Password)
}

// TestResolveConn_DesktopConnection_NonUUID_UsageError verifies that a
// non-UUID value after `desktop-connection:` returns a usage error.
func TestResolveConn_DesktopConnection_NonUUID_UsageError(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:not-a-uuid"}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-uuid")
	assert.Contains(t, err.Error(), "UUID")
	assert.Contains(t, err.Error(), "neo4j-cli desktop list")
}

// TestResolveConn_DesktopConnection_UnknownUUID_Fatals verifies that a
// well-formed but unknown UUID returns a fatal error.
func TestResolveConn_DesktopConnection_UnknownUUID_Fatals(t *testing.T) {
	const unknownUUID = "99999999-2222-3333-4444-555555555555"

	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*DesktopMatch, error) {
		require.Equal(t, unknownUUID, raw)
		return nil, &desktopErrString{
			s: "Neo4j Desktop 2 has no connection with id " + raw + ". " +
				"Run 'neo4j-cli desktop list' to see saved connections.",
		}
	})
	defer restore()

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + unknownUUID}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no connection with id "+unknownUUID)
	assert.Contains(t, err.Error(), "neo4j-cli desktop list")
}

// TestResolveConn_UnprefixedCredName_NoDesktopFallthrough verifies that an
// unprefixed --credential value never invokes the Desktop resolver seams.
func TestResolveConn_UnprefixedCredName_NoDesktopFallthrough(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restoreActive := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*DesktopMatch, error) {
		t.Fatal("unprefixed --credential must NOT invoke the desktop active-DBMS resolver")
		return nil, nil
	})
	defer restoreActive()

	restoreConn := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, _ string) (*DesktopMatch, error) {
		t.Fatal("unprefixed --credential must NOT invoke the desktop connection resolver")
		return nil, nil
	})
	defer restoreConn()

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newAdminCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=some-random-name"}))

	_, err := ResolveConn(cmd, cfg, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "some-random-name")
	assert.Contains(t, err.Error(), "credential dbms add")
}

// TestResolveConn_DesktopActive_DatabaseOverride verifies that --database can
// override the database when combined with --credential desktop.
func TestResolveConn_DesktopActive_DatabaseOverride(t *testing.T) {
	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopActiveDbmsCredentialFnForTest(func(_ context.Context, _ afero.Fs) (*DesktopMatch, error) {
		return &DesktopMatch{
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

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop", "--database=override-db"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "override-db", conn.Database, "--database must override the Desktop-supplied database")
}

// TestResolveConn_DesktopConnection_DatabaseOverride verifies that --database
// can override the database when combined with --credential desktop-connection:.
func TestResolveConn_DesktopConnection_DatabaseOverride(t *testing.T) {
	const validUUID = "11111111-2222-3333-4444-555555555555"

	t.Setenv(EnvURI, "")
	t.Setenv(EnvUsername, "")
	t.Setenv(EnvPassword, "")
	t.Setenv(EnvDatabase, "")
	t.Chdir(t.TempDir())

	restore := SetResolveDesktopConnectionCredentialFnForTest(func(_ context.Context, _ afero.Fs, raw string) (*DesktopMatch, error) {
		require.Equal(t, validUUID, raw)
		return &DesktopMatch{
			Connection: &desktopclient.Connection{
				ID:            validUUID,
				Name:          "prod-aura",
				ConnectionURI: "neo4j+s://example.databases.neo4j.io",
			},
			Creds: &desktopclient.Credentials{Username: "aura-user", Password: "aura-pw"},
		}, nil
	})
	defer restore()

	cfg, _ := newCfgWithCreds(t, "{}")
	cmd := newQueryCmd(cfg)
	require.NoError(t, cmd.ParseFlags([]string{"--credential=desktop-connection:" + validUUID, "--database=override-db"}))

	conn, err := ResolveConn(cmd, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "override-db", conn.Database, "--database must override the Desktop-supplied database")
}

// desktopErrString is a local test-only error type to construct expected
// desktop error messages in tests that need to check specific text.
type desktopErrString struct{ s string }

func (e *desktopErrString) Error() string { return e.s }
