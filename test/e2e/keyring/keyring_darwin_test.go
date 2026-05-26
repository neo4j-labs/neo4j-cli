//go:build keyring_smoke && darwin

// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package keyring_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// macOS Keychain is always available on standard macOS (including macos-latest
// CI runners), so the happy-path tests run unconditionally.
//
// # macOS Keychain isolation
//
// On macOS, the `security` binary (used by go-keyring under the hood) resolves
// the login keychain via $HOME/Library/Keychains/login.keychain-db. To isolate
// config and credentials files in a temp dir while still allowing Keychain
// access, each test symlinks the real login.keychain-db into the temp home.
// This lets neo4j-cli write config.json and credentials.json to the isolated
// temp path while the Keychain entries land in the real login keychain.
// Cleanup functions must therefore delete Keychain entries explicitly using
// credential dbms remove (via the CLI) or `security delete-generic-password`.
//
// Note: test isolation is not perfect on macOS — Keychain entries use the
// real login keychain. Tests use unique credential names to avoid collisions.

// setupDarwinHome creates a temp home directory suitable for macOS subprocess
// tests. It:
//   - Creates the neo4j-cli config dir under the temp home.
//   - Symlinks the real login.keychain-db into temp/Library/Keychains/ so
//     that the `security` binary can find it when $HOME is overridden.
//
// Returns the temp home path and the env slice for subprocess invocations.
// The test runner need not clean up the temp dir — t.TempDir() handles it.
// Keychain entries written during the test must be deleted by the caller.
func setupDarwinHome(t *testing.T) (homeDir string, env []string) {
	t.Helper()

	if os.Getenv("CI") != "true" {
		t.Skip("macOS Keychain e2e tests only run in CI (CI=true) to avoid writing to the local Keychain")
	}

	home := t.TempDir()

	// Create the neo4j-cli config directory.
	if err := os.MkdirAll(configDirForHome(home), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	// Symlink the real login.keychain-db so the security binary can find it.
	keychainsDir := filepath.Join(home, "Library", "Keychains")
	if err := os.MkdirAll(keychainsDir, 0o700); err != nil {
		t.Fatalf("mkdir Keychains dir: %v", err)
	}

	realKeychainSrc := filepath.Join(os.Getenv("HOME"), "Library", "Keychains", "login.keychain-db")
	if _, statErr := os.Stat(realKeychainSrc); statErr == nil {
		dst := filepath.Join(keychainsDir, "login.keychain-db")
		if linkErr := os.Symlink(realKeychainSrc, dst); linkErr != nil {
			t.Logf("warning: could not symlink login.keychain-db (%v); keyring operations may fail", linkErr)
		}
	} else {
		t.Logf("warning: login.keychain-db not found at %s; keyring operations may fail", realKeychainSrc)
	}

	return home, baseChildEnv(configEnvForDir(home)...)
}

// deleteKeychainEntry removes a Keychain entry by service and account name.
// Used for test cleanup. Ignores "not found" errors.
func deleteKeychainEntry(t *testing.T, service, account string) {
	t.Helper()
	out, err := exec.Command(
		"/usr/bin/security", "delete-generic-password",
		"-s", service, "-a", account,
	).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "could not be found") {
		t.Logf("warning: delete keychain entry %s/%s: %v (%s)", service, account, err, out)
	}
}

// ===========================================================================
// Happy-path tests — macOS Keychain always available
// ===========================================================================

// TestKeyring_Darwin_CredentialAddDoesNotStoreSecretInJSON verifies that in
// keyring mode, adding a dbms credential does not write the password to
// credentials.json (macOS Keychain stores it instead).
func TestKeyring_Darwin_CredentialAddDoesNotStoreSecretInJSON(t *testing.T) {
	bin := binaryPath(t)
	home, env := setupDarwinHome(t)

	// Pre-seed config with keyring mode so first-run detection is skipped.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "keyring",
	})

	exitCode, _, stderr := runCLI(t, bin,
		[]string{
			"credential", "dbms", "add",
			"--name", "darwin-smoke-add",
			"--uri", "neo4j://localhost:7687",
			"--username", "neo4j",
			"--password", "darwinsecret",
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
	assert.NotContains(t, string(data), "darwinsecret",
		"password must not be stored in credentials.json in keyring mode")

	// Clean up Keychain entries written during this test.
	t.Cleanup(func() {
		deleteKeychainEntry(t, "neo4j-cli", "dbms/darwin-smoke-add/password")
	})
}

// TestKeyring_Darwin_ForwardMigration verifies that switching from insecure to
// keyring mode migrates the password out of credentials.json into the macOS
// Keychain.
func TestKeyring_Darwin_ForwardMigration(t *testing.T) {
	bin := binaryPath(t)
	home, env := setupDarwinHome(t)

	// Pre-seed: insecure mode with a password in credentials.json.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("darwin-migrate-fwd", "darwinmigratesecret"))

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
	assert.NotContains(t, string(data), "darwinmigratesecret",
		"password must be removed from credentials.json after migration to keyring")

	// config.json must now say keyring.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "keyring", cfg["credential-storage"],
		"credential-storage must be keyring after successful migration")

	// Clean up the Keychain entry.
	t.Cleanup(func() {
		deleteKeychainEntry(t, "neo4j-cli", "dbms/darwin-migrate-fwd/password")
	})
}

// TestKeyring_Darwin_ReverseMigration verifies that switching from keyring to
// insecure mode reads the password back from the macOS Keychain and writes it
// to credentials.json.
func TestKeyring_Darwin_ReverseMigration(t *testing.T) {
	bin := binaryPath(t)
	home, env := setupDarwinHome(t)

	// Step 1: start in insecure mode with a credential.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "insecure",
	})
	writeCredentialsJSON(t, home, insecureCredentialsJSON("darwin-migrate-rev", "darwinreversesecret"))

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
	assert.Contains(t, string(data), "darwinreversesecret",
		"password must be restored to credentials.json after reverse migration")

	// config.json must say insecure.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg)
	assert.Equal(t, "insecure", cfg["credential-storage"],
		"credential-storage must be insecure after reverse migration")
}

// TestKeyring_Darwin_RemoveCleansKeyring verifies that removing a credential in
// keyring mode also removes the corresponding entry from the macOS Keychain.
//
// The indirect verification approach: add in keyring mode → remove via the CLI →
// switch to insecure mode (which succeeds cleanly with no entries to migrate,
// confirming the Keychain entry is absent).
func TestKeyring_Darwin_RemoveCleansKeyring(t *testing.T) {
	bin := binaryPath(t)
	home, env := setupDarwinHome(t)

	// Pre-seed: keyring mode.
	writeConfigJSON(t, home, map[string]interface{}{
		"credential-storage": "keyring",
	})

	// Add a credential in keyring mode.
	exitCode, _, stderr := runCLI(t, bin,
		[]string{
			"credential", "dbms", "add",
			"--name", "darwin-smoke-remove",
			"--uri", "neo4j://localhost:7687",
			"--username", "neo4j",
			"--password", "darwinremovesecret",
			"--rw",
		},
		env,
	)
	require.Equal(t, 0, exitCode, "credential dbms add must succeed; stderr=%s", stderr)

	// Remove the credential via the CLI.
	exitCode, _, stderr = runCLI(t, bin,
		[]string{"credential", "dbms", "remove", "darwin-smoke-remove", "--rw", "--yes", "--force"},
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

	// Indirect keyring probe: switch to insecure. If the Keychain entry for
	// "darwin-smoke-remove" were still present, MigrateToInsecure would have tried
	// to move it back to JSON; since the credential was removed entirely, the
	// migration should succeed cleanly with no entries to process.
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

// ===========================================================================
// Locked-Keychain tests — graceful degradation via a temporary locked Keychain
// ===========================================================================

// withLockedKeychain sets up a temporary, locked macOS Keychain as the system
// default for the duration of the test, then restores the original default on
// cleanup.
func withLockedKeychain(t *testing.T) {
	t.Helper()

	if os.Getenv("CI") != "true" {
		t.Skip("macOS Keychain e2e tests only run in CI (CI=true) to avoid modifying the local Keychain")
	}

	// Record the current default keychain so we can restore it afterwards.
	origOut, err := exec.Command("/usr/bin/security", "default-keychain").Output()
	require.NoError(t, err, "could not read current default keychain")
	// security default-keychain prints the path in double quotes.
	origPath := strings.Trim(strings.TrimSpace(string(origOut)), `"`)

	// Create a temporary keychain with an empty passphrase.
	kcDir := t.TempDir()
	kcPath := kcDir + "/smoke.keychain-db"
	err = exec.Command("/usr/bin/security", "create-keychain", "-p", "", kcPath).Run()
	require.NoError(t, err, "could not create temporary keychain")

	// Set it as the default keychain.
	err = exec.Command("/usr/bin/security", "default-keychain", "-s", kcPath).Run()
	require.NoError(t, err, "could not set temporary keychain as default")

	// Lock it so all operations on it fail with an authentication error.
	err = exec.Command("/usr/bin/security", "lock-keychain", kcPath).Run()
	require.NoError(t, err, "could not lock temporary keychain")

	t.Cleanup(func() {
		// Restore the original default keychain.
		_ = exec.Command("/usr/bin/security", "default-keychain", "-s", origPath).Run()
		// Remove the temp keychain from the search list (best-effort).
		_ = exec.Command("/usr/bin/security", "delete-keychain", kcPath).Run()
	})
}

// TestKeyring_Darwin_LockedKeychain_FirstRun_WritesKeyringDefault verifies the
// first-run behaviour with a locked default Keychain.
//
// On macOS, ProbeKeyringAvailability uses `security find-generic-password` which
// searches ALL keychains in the current search list, not only the default. If
// the unlocked login.keychain-db is still reachable (e.g. via the symlink in the
// temp home), the probe returns ErrNotFound (= keyring is available) and the CLI
// writes `credential-storage: keyring` silently.
//
// This is different from Linux (where a missing D-Bus causes the probe to fail
// completely) and reflects macOS's per-keychain locking model: locking the
// default only blocks writes to it, not reads from other keychains in the
// search list. The write-failure is caught at migration time (see
// TestKeyring_Darwin_LockedKeychain_ConfigSet_Fails).
func TestKeyring_Darwin_LockedKeychain_FirstRun_WritesKeyringDefault(t *testing.T) {
	withLockedKeychain(t)

	bin := binaryPath(t)
	home, env := setupDarwinHome(t)

	// Run any command that triggers PersistentPreRunE. "config list" is the
	// simplest leaf command that reliably runs PersistentPreRunE and exits 0.
	exitCode, _, stderr := runCLI(t, bin, []string{"config", "list"}, env)
	require.Equal(t, 0, exitCode, "config list must exit 0; stderr=%s", stderr)

	// config.json must exist after first run.
	cfg := readConfigJSON(t, home)
	require.NotNil(t, cfg, "config.json must exist after first run")

	// On macOS, the probe succeeds even with a locked default keychain (because
	// find-generic-password searches the full search list). The CLI therefore
	// writes keyring as the default without a warning.
	assert.Equal(t, "keyring", cfg["credential-storage"],
		"credential-storage should be keyring (probe passes with locked default but unlocked search-list keychain)")

	// No warning expected — the probe didn't fail.
	assert.NotContains(t, stderr, "Warning: OS keyring is unavailable",
		"no warning expected when probe passes; stderr=%s", stderr)
}

// TestKeyring_Darwin_LockedKeychain_ConfigSet_Fails is skipped because the
// locked-default-keychain approach does not reliably reproduce a Keychain write
// failure in subprocess tests.
//
// When `security default-keychain -s <locked>` changes the default, the security
// binary's search list still includes the unlocked login.keychain-db (symlinked
// into the temp home via setupDarwinHome). The `add-generic-password -U` command
// may write to that unlocked keychain instead of failing. As a result, the
// migration succeeds even though the default keychain is locked.
//
// The Keychain-unavailable error path is fully covered by MockInitWithError unit
// tests in common/clicfg/credentials and neo4j-cli/app.
func TestKeyring_Darwin_LockedKeychain_ConfigSet_Fails(t *testing.T) {
	t.Skip("locked-default-keychain approach does not reliably reproduce write failure when unlocked login.keychain-db is in the search list — covered by MockInitWithError unit tests")
}

// ===========================================================================
// Missing-security tests — PATH-stub approach
//
// NOTE: go-keyring hardcodes the path to the `security` binary as
// /usr/bin/security (see keyring_darwin.go const execPathKeychain). Because
// the absolute path bypasses PATH resolution entirely, a PATH-prepend stub
// cannot intercept the calls. These tests are therefore skipped with an
// explanatory message. The graceful-degradation behaviour they would cover is
// exercised at the unit-test level via MockInitWithError in
// common/clicfg/credentials and neo4j-cli/app tests.
// ===========================================================================

// TestKeyring_Darwin_MissingSecurity_FirstRun_WritesInsecureAndWarns is skipped
// because go-keyring hardcodes /usr/bin/security, making a PATH stub
// ineffective. See the missing-security section comment above.
func TestKeyring_Darwin_MissingSecurity_FirstRun_WritesInsecureAndWarns(t *testing.T) {
	t.Skip("go-keyring hardcodes /usr/bin/security; PATH-stub approach not applicable — covered by MockInitWithError unit tests")
}

// TestKeyring_Darwin_MissingSecurity_ConfigSet_Fails is skipped because
// go-keyring hardcodes /usr/bin/security, making a PATH stub ineffective.
// See the missing-security section comment above.
func TestKeyring_Darwin_MissingSecurity_ConfigSet_Fails(t *testing.T) {
	t.Skip("go-keyring hardcodes /usr/bin/security; PATH-stub approach not applicable — covered by MockInitWithError unit tests")
}
