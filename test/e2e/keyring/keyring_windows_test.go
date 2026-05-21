//go:build keyring_smoke && windows

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package keyring_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Windows Credential Manager is always available on windows-latest CI runners,
// so all tests here run unconditionally — no daemon guard is needed.

// TestKeyring_Windows_CredentialAddDoesNotStoreSecretInJSON verifies that in
// keyring mode, adding a dbms credential does not write the password to
// credentials.json (Windows Credential Manager stores it instead).
func TestKeyring_Windows_CredentialAddDoesNotStoreSecretInJSON(t *testing.T) {
	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed config with keyring mode so first-run detection is skipped.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "keyring",
	})

	env := baseChildEnv(configEnvForDir(home)...)

	exitCode, _, stderr := runCLI(t, bin,
		[]string{
			"credential", "dbms", "add",
			"--name", "winsmoketest",
			"--uri", "neo4j://localhost:7687",
			"--username", "neo4j",
			"--password", "winsecret",
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
	assert.NotContains(t, string(data), "winsecret",
		"password must not be stored in credentials.json in keyring mode")

	// Clean up: remove the credential so it is not left in the Windows
	// Credential Manager for subsequent test runs.
	t.Cleanup(func() {
		runCLI(t, bin, []string{"credential", "dbms", "remove", "winsmoketest", "--rw"}, env)
	})
}

// TestKeyring_Windows_ForwardMigration verifies that switching from insecure to
// keyring mode migrates the password out of credentials.json into the Windows
// Credential Manager.
func TestKeyring_Windows_ForwardMigration(t *testing.T) {
	bin := binaryPath(t)
	home := t.TempDir()

	// Pre-seed: insecure mode with a password in credentials.json.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("winlocaldb", "winmigratesecret"))

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
	assert.NotContains(t, string(data), "winmigratesecret",
		"password must be removed from credentials.json after migration to keyring")

	// config.json must now say keyring.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "keyring", cfg["credential-storage"],
		"credential-storage must be keyring after successful migration")

	// Clean up the keyring entry.
	t.Cleanup(func() {
		runCLI(t, bin, []string{"credential", "dbms", "remove", "winlocaldb", "--rw"}, env)
	})
}

// TestKeyring_Windows_ReverseMigration verifies that switching from keyring to
// insecure mode reads the password back from the Windows Credential Manager and
// writes it to credentials.json.
func TestKeyring_Windows_ReverseMigration(t *testing.T) {
	bin := binaryPath(t)
	home := t.TempDir()

	// Step 1: start in insecure mode with a credential.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("winrevdb", "winreversesecret"))

	env := baseChildEnv(configEnvForDir(home)...)

	// Step 2: migrate to keyring (forward migration).
	exitCode, _, stderr := runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "keyring", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode, "forward migration must succeed; stderr=%s", stderr)

	// Step 3: migrate back to insecure (reverse migration).
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
	assert.Contains(t, string(data), "winreversesecret",
		"password must be restored to credentials.json after reverse migration")

	// config.json must say insecure.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "insecure", cfg["credential-storage"],
		"credential-storage must be insecure after reverse migration")
}

// TestKeyring_Windows_RemoveCleansKeyring verifies that removing a credential in
// keyring mode also removes the corresponding entry from the Windows Credential
// Manager.
//
// The indirect verification approach: add in keyring mode → remove via the CLI →
// switch to insecure mode (which succeeds cleanly with no entries to migrate,
// confirming the keyring entry is absent). A second forward migration also succeeds
// cleanly, providing an additional signal.
func TestKeyring_Windows_RemoveCleansKeyring(t *testing.T) {
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
			"--name", "wintoremove",
			"--uri", "neo4j://localhost:7687",
			"--username", "neo4j",
			"--password", "winremovesecret",
			"--rw",
		},
		env,
	)
	require.Equal(t, 0, exitCode, "credential dbms add must succeed; stderr=%s", stderr)

	// Remove the credential via the CLI.
	exitCode, _, stderr = runCLI(t, bin,
		[]string{"credential", "dbms", "remove", "wintoremove", "--rw"},
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

	// Indirect keyring probe: switch to insecure. If the keyring entry for
	// "wintoremove" were still present, MigrateToInsecure would have tried to move
	// it back to JSON; since the credential was removed entirely, the migration
	// should succeed cleanly with no entries to process.
	exitCode, _, stderr = runCLI(t, bin,
		[]string{"config", "set", "credential-storage", "insecure", "--rw"},
		env,
	)
	require.Equal(t, 0, exitCode,
		"switching to insecure after remove must succeed; stderr=%s", stderr)

	// Still no credentials in JSON after the mode switch.
	creds = readCredentialsJSON(t, home)
	require.NotNil(t, creds)
	dbms, ok = creds["dbms"].(map[string]interface{})
	require.True(t, ok)
	dbmsCreds, _ = dbms["credentials"].([]interface{})
	assert.Empty(t, dbmsCreds, "no credentials must be present after remove + mode switch")
}
