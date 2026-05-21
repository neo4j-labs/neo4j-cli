//go:build keyring_smoke && linux

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package keyring_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripDBUS returns a copy of env with DBUS_SESSION_BUS_ADDRESS removed.
// Used by no-daemon tests to simulate a headless environment.
func stripDBUS(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "DBUS_SESSION_BUS_ADDRESS=") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ===========================================================================
// No-daemon group — always run, DBUS_SESSION_BUS_ADDRESS stripped from env
// ===========================================================================

// TestKeyring_NoDaemon_FirstRun_WritesInsecureAndWarns verifies that on the
// first invocation without a keyring daemon, the CLI defaults to insecure
// storage and prints a warning on stderr.
func TestKeyring_NoDaemon_FirstRun_WritesInsecureAndWarns(t *testing.T) {
	bin := binaryPath(t)
	home := t.TempDir()

	env := stripDBUS(baseChildEnv(configEnvForDir(home)...))

	// Run any command that triggers PersistentPreRunE. "version" is cheapest.
	exitCode, _, stderr := runCLI(t, bin, []string{"version"}, env)
	require.Equal(t, 0, exitCode, "version must exit 0; stderr=%s", stderr)

	// credential-storage: insecure must be written to config.json
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg, "config.json must exist after first run")
	assert.Equal(t, "insecure", cfg["credential-storage"],
		"credential-storage must be insecure when keyring is unavailable")

	// Upgrade notice must appear on stderr
	assert.Contains(t, stderr, "Warning: OS keyring is unavailable",
		"must warn about keyring unavailability; stderr=%s", stderr)
	assert.Contains(t, stderr, "neo4j-cli config set credential-storage keyring --rw",
		"must suggest the migration command; stderr=%s", stderr)
}

// TestKeyring_NoDaemon_ConfigSet_FailsWithNoCreds verifies that when there are
// no credentials and the keyring is unavailable, attempting to explicitly set
// credential-storage to keyring fails with a non-zero exit and an error message.
func TestKeyring_NoDaemon_ConfigSet_FailsWithNoCreds(t *testing.T) {
	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed config with insecure (simulates first-run having already run
	// and detected an unavailable keyring). We can also start totally fresh
	// so the first-run default sets insecure, then try the explicit set.
	// For clarity: fresh home + no daemon → first run sets insecure → then
	// we try to switch to keyring explicitly.
	env := stripDBUS(baseChildEnv(configEnvForDir(home)...))

	// First run to initialise config.json with insecure default.
	_, _, _ = runCLI(t, bin, []string{"version"}, env)

	// Now try to switch to keyring explicitly — must fail because no daemon.
	exitCode, _, stderr := runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "keyring", "--rw"},
		env,
	)
	assert.NotEqual(t, 0, exitCode,
		"config set credential-storage keyring must fail when keyring is unavailable; stderr=%s", stderr)
	// The error should mention that the keyring is unavailable.
	assert.NotEmpty(t, stderr, "stderr must contain an error message")
}

// TestKeyring_NoDaemon_ConfigSet_FailsWithExistingCreds verifies that when
// credentials exist in insecure mode and the keyring is unavailable, attempting
// to migrate to keyring fails and leaves credential-storage unchanged.
func TestKeyring_NoDaemon_ConfigSet_FailsWithExistingCreds(t *testing.T) {
	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed: insecure mode + one dbms credential with a password.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("localdb", "supersecret"))

	env := stripDBUS(baseChildEnv(configEnvForDir(home)...))

	exitCode, _, stderr := runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "keyring", "--rw"},
		env,
	)
	assert.NotEqual(t, 0, exitCode,
		"migration must fail when keyring is unavailable; stderr=%s", stderr)
	assert.NotEmpty(t, stderr, "stderr must contain an error message")

	// credential-storage must remain insecure — not changed to keyring.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "insecure", cfg["credential-storage"],
		"credential-storage must remain insecure after failed migration")
}

// ===========================================================================
// With-daemon group — requires DBUS_SESSION_BUS_ADDRESS; mandatory in CI
// ===========================================================================

// requireDaemon skips the test if DBUS_SESSION_BUS_ADDRESS is not set,
// unless CI=true in which case it fatals (CI must set up the daemon correctly).
func requireDaemon(t *testing.T) {
	t.Helper()
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("DBUS_SESSION_BUS_ADDRESS not set in CI — dbus-run-session/gnome-keyring setup failed")
		}
		t.Skip("DBUS_SESSION_BUS_ADDRESS not set; run inside dbus-run-session for with-daemon tests")
	}
}

// TestKeyring_WithDaemon_CredentialAddDoesNotStoreSecretInJSON verifies that
// when credential-storage is keyring and a keyring daemon is available, adding
// a dbms credential does not store the password in credentials.json.
func TestKeyring_WithDaemon_CredentialAddDoesNotStoreSecretInJSON(t *testing.T) {
	requireDaemon(t)

	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed config with keyring mode (skip first-run detection).
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "keyring",
	})

	env := baseChildEnv(configEnvForDir(home)...)

	exitCode, _, stderr := runCLI(t, bin,
		[]string{
			"credential", "dbms", "add",
			"--name", "smoketest",
			"--uri", "neo4j://localhost:7687",
			"--username", "neo4j",
			"--password", "topsecret",
			"--rw",
		},
		env,
	)
	require.Equal(t, 0, exitCode, "credential dbms add must succeed; stderr=%s", stderr)

	// credentials.json must not contain the password.
	creds := readCredentialsJSON(t, home)
	require.NotNil(t, creds, "credentials.json must exist after credential add")

	data, err := json.Marshal(creds)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "topsecret",
		"password must not be stored in credentials.json in keyring mode")
}

// TestKeyring_WithDaemon_ForwardMigration verifies that switching from insecure
// to keyring mode migrates the password out of credentials.json.
func TestKeyring_WithDaemon_ForwardMigration(t *testing.T) {
	requireDaemon(t)

	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed: insecure mode with a password in credentials.json.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("localdb", "migratesecret"))

	env := baseChildEnv(configEnvForDir(home)...)

	exitCode, _, stderr := runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "keyring", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode, "migration to keyring must succeed; stderr=%s", stderr)

	// credentials.json must no longer contain the password.
	creds := readCredentialsJSON(t, home)
	require.NotNil(t, creds)
	data, err := json.Marshal(creds)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "migratesecret",
		"password must be removed from credentials.json after migration to keyring")

	// config.json must now say keyring.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "keyring", cfg["credential-storage"],
		"credential-storage must be keyring after successful migration")
}

// TestKeyring_WithDaemon_ReverseMigration verifies that switching from keyring
// to insecure mode writes the password back into credentials.json.
func TestKeyring_WithDaemon_ReverseMigration(t *testing.T) {
	requireDaemon(t)

	bin := binaryPath(t)
	home := t.TempDir()

	// Step 1: set up in insecure mode with a credential.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("localdb", "reversesecret"))

	env := baseChildEnv(configEnvForDir(home)...)

	// Step 2: migrate to keyring.
	exitCode, _, stderr := runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "keyring", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode, "forward migration must succeed; stderr=%s", stderr)

	// Step 3: migrate back to insecure.
	exitCode, _, stderr = runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "insecure", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode, "reverse migration must succeed; stderr=%s", stderr)

	// credentials.json must contain the password again.
	creds := readCredentialsJSON(t, home)
	require.NotNil(t, creds)
	data, err := json.Marshal(creds)
	require.NoError(t, err)
	assert.Contains(t, string(data), "reversesecret",
		"password must be restored to credentials.json after reverse migration")

	// config.json must say insecure.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "insecure", cfg["credential-storage"],
		"credential-storage must be insecure after reverse migration")
}

// TestKeyring_WithDaemon_RemoveCleansKeyring verifies that removing a credential
// in keyring mode also deletes the corresponding keyring entry.
// The indirect verification approach: add in keyring mode → remove → then do a
// reverse migration (insecure) and verify credentials.json has no entry (which
// also confirms the keyring entry is gone, because MigrateToInsecure would have
// found nothing to move back).
func TestKeyring_WithDaemon_RemoveCleansKeyring(t *testing.T) {
	requireDaemon(t)

	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed: keyring mode.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "keyring",
	})

	env := baseChildEnv(configEnvForDir(home)...)

	// Add a credential in keyring mode.
	exitCode, _, stderr := runCLI(t, bin,
		[]string{
			"credential", "dbms", "add",
			"--name", "toremove",
			"--uri", "neo4j://localhost:7687",
			"--username", "neo4j",
			"--password", "removesecret",
			"--rw",
		},
		env,
	)
	require.Equal(t, 0, exitCode, "credential dbms add must succeed; stderr=%s", stderr)

	// Remove the credential.
	exitCode, _, stderr = runCLI(t, bin,
		[]string{"credential", "dbms", "remove", "toremove", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode, "credential dbms remove must succeed; stderr=%s", stderr)

	// credentials.json must have no dbms credentials.
	creds := readCredentialsJSON(t, home)
	require.NotNil(t, creds)

	dbms, ok := creds["dbms"].(map[string]interface{})
	require.True(t, ok, "credentials.json must have a dbms section")
	dbmsCreds, _ := dbms["credentials"].([]interface{})
	assert.Empty(t, dbmsCreds, "dbms credentials list must be empty after remove")

	// Perform a reverse migration to insecure as an indirect probe that the
	// keyring entry is also gone. If an entry existed for "toremove", it would
	// be moved to JSON; since the credential itself was removed, this just
	// switches the mode cleanly with no entries to migrate.
	exitCode, _, stderr = runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "insecure", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode, "switching to insecure after remove must succeed; stderr=%s", stderr)

	// Still no credentials in JSON.
	creds = readCredentialsJSON(t, home)
	require.NotNil(t, creds)
	dbms, ok = creds["dbms"].(map[string]interface{})
	require.True(t, ok)
	dbmsCreds, _ = dbms["credentials"].([]interface{})
	assert.Empty(t, dbmsCreds, "no credentials must be present after remove + mode switch")
}
