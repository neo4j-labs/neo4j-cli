// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errProviderFail is a sentinel returned by failAfterNProvider to simulate
// a keyring.Set failure.
var errProviderFail = errors.New("simulated keyring provider failure")

// newKeyringTestCredentials creates a Credentials instance in keyring mode
// backed by a mock keyring provider. The caller can pre-populate the mock
// before calling this helper.
func newKeyringTestCredentials(t *testing.T, mock *mockKeyringProvider, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	credentials.SetKeyringProviderForTest(t, mock)
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard))
	return creds, fs
}

// readCredentialsJSON reads the credentials.json file from the memfs and
// returns the raw bytes. Helpers should use this to assert on-disk state.
func readCredentialsJSON(t *testing.T, fs afero.Fs) []byte {
	t.Helper()
	p := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	data, err := afero.ReadFile(fs, p)
	require.NoError(t, err)
	return data
}

// TestSetStorageMode_InsecureIsNoOp verifies that switching to insecure mode
// does not touch the keyring.
func TestSetStorageMode_InsecureIsNoOp(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"","token-expiry":0}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeInsecure, io.Discard))

	// Mock must not have been touched
	_, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "insecure mode must not touch the keyring")

	// In-memory value should still be populated from JSON
	assert.Equal(t, "s3cr3t", creds.Aura.Credentials[0].ClientSecret)
}

// TestSetStorageMode_KeyringMode_LoadsFromKeyring verifies that switching to
// keyring mode populates in-memory sensitive fields from the keyring,
// overwriting whatever the JSON loaded.
func TestSetStorageMode_KeyringMode_LoadsFromKeyring(t *testing.T) {
	mock := newMockKeyringProvider()
	// Pre-populate the keyring with a known value that differs from JSON
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "keyring-secret"))

	creds, _ := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"json-secret","access-token":"","token-expiry":0}]}}`)

	assert.Equal(t, "keyring-secret", creds.Aura.Credentials[0].ClientSecret,
		"keyring value must overwrite JSON value")
}

// TestSetStorageMode_KeyringMode_MissingRequired_Error verifies that when
// keyring mode is active and a required field is missing from both keyring and
// JSON, SetStorageMode succeeds (no error) but leaves the field empty so that
// the command can still run. The missing-credential warning is written to
// stderr. Covers aura client-secret and dbms password.
func TestSetStorageMode_KeyringMode_MissingRequired_Warn(t *testing.T) {
	tests := []struct {
		name         string
		credJSON     string
		checkEmptyFn func(creds *credentials.Credentials) string
	}{
		{
			name:     "aura missing client-secret",
			credJSON: `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`,
			checkEmptyFn: func(creds *credentials.Credentials) string {
				return creds.Aura.Credentials[0].ClientSecret
			},
		},
		{
			name:     "dbms missing password",
			credJSON: `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			checkEmptyFn: func(creds *credentials.Credentials) string {
				return creds.Dbms.Credentials[0].Password
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockKeyringProvider()
			credentials.SetKeyringProviderForTest(t, mock)

			fs, err := testfs.GetTestFs("{}", tc.credJSON)
			require.NoError(t, err)

			creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
			var warnBuf bytes.Buffer
			err = creds.SetStorageMode(credentials.StorageModeKeyring, &warnBuf)
			require.NoError(t, err, "missing required field must warn but not error")
			assert.Equal(t, "", tc.checkEmptyFn(creds), "field must remain empty when missing from both keyring and JSON")
			assert.Contains(t, warnBuf.String(), "Warning:", "missing required field must write a warning")
		})
	}
}

// TestSetStorageMode_KeyringMode_NonErrNotFound_Warns verifies that when
// loadCredFromKeyring encounters a non-ErrNotFound error (e.g. keyring daemon
// crash, permission denied), a warning is written to warnW and processing
// stops for that credential. SetStorageMode still returns nil (warn-and-continue).
func TestSetStorageMode_KeyringMode_NonErrNotFound_Warns(t *testing.T) {
	mock := newMockKeyringProvider()
	// Seed a value so Get would normally succeed; errorOnGetProvider will
	// override it to return a non-ErrNotFound error for the client-secret key.
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))

	failingProvider := &errorOnGetProvider{
		inner:   mock,
		failKey: credentials.KeyringKey("aura", "prod", "client-secret"),
	}
	credentials.SetKeyringProviderForTest(t, failingProvider)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	var warnBuf bytes.Buffer
	err = creds.SetStorageMode(credentials.StorageModeKeyring, &warnBuf)
	require.NoError(t, err, "non-ErrNotFound keyring error must not cause SetStorageMode to fail")
	assert.Contains(t, warnBuf.String(), "Warning: keyring read failed for aura/prod/client-secret")
}

// TestSetStorageMode_KeyringMode_PreMigrationFallback verifies that when
// keyring mode is active but a required field has no keyring entry yet
// (pre-migration state), the JSON value is used silently.
func TestSetStorageMode_KeyringMode_PreMigrationFallback(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	// JSON holds the secret; keyring is empty → pre-migration fallback
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"json-secret","access-token":"","token-expiry":0}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	err = creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard)
	require.NoError(t, err, "pre-migration fallback must not error")
	assert.Equal(t, "json-secret", creds.Aura.Credentials[0].ClientSecret,
		"JSON value must be used when keyring has no entry")
}

// TestSetStorageMode_KeyringMode_AutoMigration_Success verifies REQ-F-019: when
// keyring mode is active and a field has ErrNotFound in keyring but the JSON
// value is non-empty, SetStorageMode succeeds (REQ-F-016) AND attempts
// auto-migration: the value is written to the keyring and credentials.json is
// scrubbed (sensitive field absent from JSON after SetStorageMode returns).
func TestSetStorageMode_KeyringMode_AutoMigration_Success(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	// JSON holds the password; keyring is empty → auto-migration path
	fs, err := testfs.GetTestFs("{}", `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"json-pass","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard))

	// In-memory value must still be populated
	assert.Equal(t, "json-pass", creds.Dbms.Credentials[0].Password,
		"in-memory password must remain available after auto-migration")

	// Keyring must now hold the value (auto-migration fired)
	val, getErr := mock.Get(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password"))
	require.NoError(t, getErr, "keyring must hold the auto-migrated password")
	assert.Equal(t, "json-pass", val)

	// credentials.json must no longer contain the plaintext password
	data := readCredentialsJSON(t, fs)
	assert.NotContains(t, string(data), "json-pass",
		"credentials.json must be scrubbed after successful auto-migration")
}

// TestSetStorageMode_KeyringMode_AutoMigration_KeyringSetFails verifies REQ-F-019:
// when the keyring.Set call during auto-migration fails, SetStorageMode still
// succeeds (in-memory value available), the JSON is NOT scrubbed, and no error
// is returned. Retry happens on the next command invocation.
func TestSetStorageMode_KeyringMode_AutoMigration_KeyringSetFails(t *testing.T) {
	// failAfterNProvider with failAfter=0 fails every Set call immediately.
	// Get still succeeds (returns ErrNotFound for unknown keys), so the
	// pre-migration fallback check passes and we reach the Set path.
	alwaysFailSet := &failAfterNProvider{inner: newMockKeyringProvider(), failAfter: 0}
	credentials.SetKeyringProviderForTest(t, alwaysFailSet)

	// JSON holds the secret; keyring is empty → auto-migration attempted but Set fails
	fs, err := testfs.GetTestFs("{}", `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"json-pass","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	// SetStorageMode must succeed despite the keyring.Set failure
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard),
		"SetStorageMode must succeed even when auto-migration keyring.Set fails")

	// In-memory value must still be populated (JSON fallback used)
	assert.Equal(t, "json-pass", creds.Dbms.Credentials[0].Password,
		"in-memory password must be available via JSON fallback when auto-migration fails")

	// credentials.json must still contain the password (no scrub on failed migration)
	data := readCredentialsJSON(t, fs)
	assert.Contains(t, string(data), "json-pass",
		"credentials.json must not be modified when auto-migration keyring.Set fails")
}

// TestMigrateToInsecure_JSONFallbackValue_SucceedsWithoutKeyringEntry verifies
// REQ-F-018: during reverse migration, if keyring.Get returns ErrNotFound for
// a required field but the in-memory value is non-empty (populated via the
// REQ-F-016 JSON fallback during load), the migration succeeds — the field is
// treated as "already in JSON". The save() at the end persists all in-memory
// values to JSON.
func TestMigrateToInsecure_JSONFallbackValue_SucceedsWithoutKeyringEntry(t *testing.T) {
	mock := newMockKeyringProvider()
	// Seed only the optional access-token; leave the required client-secret absent
	// from the keyring to simulate a credential whose secret was never migrated
	// to the keyring (REQ-F-016 JSON-fallback state).
	// But first we need SetStorageMode to succeed — for that we need either the
	// keyring entry OR the JSON value present. We keep the client-secret in JSON.
	credentials.SetKeyringProviderForTest(t, mock)

	// JSON has the client-secret; keyring is empty → SetStorageMode uses JSON fallback
	creds, fs := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"json-secret","access-token":"","token-expiry":0}]}}`)

	// The in-memory client-secret is now "json-secret" (from JSON fallback).
	// Keyring has no entry for it (mock is empty for client-secret).
	assert.Equal(t, "json-secret", creds.Aura.Credentials[0].ClientSecret,
		"pre-condition: in-memory must hold the JSON-fallback value")

	// MigrateToInsecure should succeed: ErrNotFound for client-secret + in-memory non-empty
	require.NoError(t, creds.MigrateToInsecure(),
		"MigrateToInsecure must succeed when required field is only in JSON (REQ-F-018)")

	// The secret must be present in credentials.json after migration
	data := readCredentialsJSON(t, fs)
	assert.Contains(t, string(data), "json-secret",
		"credentials.json must contain the client-secret after reverse migration")
}

// TestMigrateToInsecure_JSONFallbackEmpty_StillErrors verifies that REQ-F-018
// does NOT change the hard-error behavior when both the keyring entry AND the
// in-memory value are absent/empty for a required field.
func TestMigrateToInsecure_JSONFallbackEmpty_StillErrors(t *testing.T) {
	mock := newMockKeyringProvider()
	// Seed the required client-secret in keyring so SetStorageMode succeeds
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	creds, _ := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`)

	// After SetStorageMode the in-memory value is "s3cr3t" (from keyring). Now
	// delete the keyring entry and zero the in-memory field to simulate a state
	// where both are absent before calling MigrateToInsecure.
	require.NoError(t, mock.Delete(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret")))
	creds.Aura.Credentials[0].ClientSecret = ""

	err := creds.MigrateToInsecure()
	require.Error(t, err, "must error when both keyring and in-memory value are absent")
	assert.Contains(t, err.Error(), "prod", "error must name the credential")
}

// TestSave_KeyringMode_SensitiveFieldsRoutedToKeyring verifies that in keyring
// mode, save() writes empty strings for sensitive fields in credentials.json and
// stores the real values in the keyring. Covers aura client-secret, dbms
// password, and embed api-key.
func TestSave_KeyringMode_SensitiveFieldsRoutedToKeyring(t *testing.T) {
	tests := []struct {
		name        string
		addCred     func(creds *credentials.Credentials) error
		secretValue string
		keyringKey  string
	}{
		{
			name: "aura client-secret",
			addCred: func(creds *credentials.Credentials) error {
				return creds.Aura.Add("prod", "id1", "s3cr3t")
			},
			secretValue: "s3cr3t",
			keyringKey:  credentials.KeyringKey("aura", "prod", "client-secret"),
		},
		{
			name: "dbms password",
			addCred: func(creds *credentials.Credentials) error {
				return creds.Dbms.Add("local", "neo4j", "p4ssword", "neo4j", "bolt://localhost:7687")
			},
			secretValue: "p4ssword",
			keyringKey:  credentials.KeyringKey("dbms", "local", "password"),
		},
		{
			name: "embed api-key",
			addCred: func(creds *credentials.Credentials) error {
				return creds.Embed.Add("openai", "openai", "text-embedding-ada-002", "", "sk-key123", 1536)
			},
			secretValue: "sk-key123",
			keyringKey:  credentials.KeyringKey("embed", "openai", "api-key"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockKeyringProvider()
			credentials.SetKeyringProviderForTest(t, mock)

			fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
			require.NoError(t, err)

			creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
			require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard))
			require.NoError(t, tc.addCred(creds))

			data := readCredentialsJSON(t, fs)
			assert.NotContains(t, string(data), tc.secretValue, "credentials.json must not contain sensitive field in keyring mode")

			val, err := mock.Get(credentials.ServiceName, tc.keyringKey)
			require.NoError(t, err)
			assert.Equal(t, tc.secretValue, val)
		})
	}
}

// TestSave_KeyringMode_InMemoryValuesPreserved verifies that after save() in
// keyring mode, the in-memory credential still holds the sensitive values so
// the current process can continue using them without re-reading from keyring.
func TestSave_KeyringMode_InMemoryValuesPreserved(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard))
	require.NoError(t, creds.Aura.Add("prod", "id1", "s3cr3t"))

	// In-memory value must still be set
	assert.Equal(t, "s3cr3t", creds.Aura.Credentials[0].ClientSecret,
		"in-memory client secret must remain after keyring save")
}

// TestSave_InsecureMode_WritesAllFields verifies that in insecure mode the
// credentials are written in full (unchanged behaviour).
func TestSave_InsecureMode_WritesAllFields(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Aura.Add("prod", "id1", "s3cr3t"))

	data := readCredentialsJSON(t, fs)
	assert.Contains(t, string(data), "s3cr3t", "insecure mode must write sensitive field to JSON")
}

// TestSave_KeyringMode_KeyringSetError_ReturnsError verifies that a keyring.Set
// failure surfaces as an error from the mutating method (e.g. Add) rather than
// crashing the process with a panic.
func TestSave_KeyringMode_KeyringSetError_ReturnsError(t *testing.T) {
	// failAfter: 0 → every Set call fails immediately.
	alwaysFail := &failAfterNProvider{inner: newMockKeyringProvider(), failAfter: 0}
	credentials.SetKeyringProviderForTest(t, alwaysFail)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	// SetStorageMode loads from keyring (Get only, no Set); zero credentials → no
	// keyring reads, so the failing Set provider doesn't matter here.
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring, io.Discard))

	before := readCredentialsJSON(t, fs)

	addErr := creds.Dbms.Add("local", "neo4j", "p4ss", "neo4j", "bolt://localhost:7687")
	require.Error(t, addErr, "keyring.Set failure must propagate as an error, not a panic")
	assert.Contains(t, addErr.Error(), "keyring set")

	// credentials.json must remain unchanged (no partial scrub)
	after := readCredentialsJSON(t, fs)
	assert.Equal(t, string(before), string(after), "credentials.json must not be modified when keyring write fails")
}

// TestKeyringMode_LoadReload verifies that credentials saved in keyring mode
// are correctly loaded in a new Credentials instance using the same mock.
func TestKeyringMode_LoadReload(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	// First instance: add credential in keyring mode
	creds1 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds1.SetStorageMode(credentials.StorageModeKeyring, io.Discard))
	require.NoError(t, creds1.Aura.Add("prod", "id1", "s3cr3t"))

	// Verify JSON is scrubbed
	data := readCredentialsJSON(t, fs)
	var file struct {
		Aura struct {
			Credentials []struct {
				Name         string `json:"name"`
				ClientSecret string `json:"client-secret"`
			} `json:"credentials"`
		} `json:"aura"`
	}
	require.NoError(t, json.Unmarshal(data, &file))
	require.Len(t, file.Aura.Credentials, 1)
	assert.Equal(t, "", file.Aura.Credentials[0].ClientSecret, "JSON must have empty client-secret")

	// Second instance: load same FS — should get secret from keyring
	creds2 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds2.SetStorageMode(credentials.StorageModeKeyring, io.Discard))
	require.Len(t, creds2.Aura.Credentials, 1)
	assert.Equal(t, "s3cr3t", creds2.Aura.Credentials[0].ClientSecret,
		"reloaded credential must have secret from keyring")
}

// TestStorageMode_DefaultIsInsecure verifies that a fresh Credentials instance
// defaults to insecure mode, preserving backwards compatibility.
func TestStorageMode_DefaultIsInsecure(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	assert.Equal(t, credentials.StorageModeInsecure, creds.StorageMode())
}

// --- MigrateToKeyring tests ---

// newInsecureTestCredentials creates a Credentials instance in insecure mode
// (the default) backed by a mock keyring provider. The credentialsJSON is
// loaded into an in-memory FS.
func newInsecureTestCredentials(t *testing.T, mock *mockKeyringProvider, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	credentials.SetKeyringProviderForTest(t, mock)
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	// default mode is insecure — no SetStorageMode call needed
	return creds, fs
}

// TestMigrateToKeyring_Success verifies the happy path: all secrets are written
// to the keyring, zeroed in in-memory structs, and saved (scrubbed) to JSON.
func TestMigrateToKeyring_Success(t *testing.T) {
	mock := newMockKeyringProvider()
	creds, fs := newInsecureTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"tok","token-expiry":0}]},`+
			`"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"p4ss","database-name":"neo4j","uri":"bolt://localhost:7687"}]},`+
			`"embed":{"credentials":[{"name":"openai","provider":"openai","model":"ada","base-url":"","dimensions":1536,"api-key":"sk-key"}]}}`)

	require.NoError(t, creds.MigrateToKeyring())

	// Keyring must hold all secrets
	val, err := mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", val)

	val, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"))
	require.NoError(t, err)
	assert.Equal(t, "tok", val)

	val, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password"))
	require.NoError(t, err)
	assert.Equal(t, "p4ss", val)

	val, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("embed", "openai", "api-key"))
	require.NoError(t, err)
	assert.Equal(t, "sk-key", val)

	// In-memory sensitive fields must be zeroed
	assert.Equal(t, "", creds.Aura.Credentials[0].ClientSecret, "in-memory ClientSecret must be zeroed")
	assert.Equal(t, "", creds.Aura.Credentials[0].AccessToken, "in-memory AccessToken must be zeroed")
	assert.Equal(t, "", creds.Dbms.Credentials[0].Password, "in-memory Password must be zeroed")
	assert.Equal(t, "", creds.Embed.Credentials[0].APIKey, "in-memory APIKey must be zeroed")

	// storageMode must be keyring after MigrateToKeyring (owned internally, no SetStorageMode call needed)
	assert.Equal(t, credentials.StorageModeKeyring, creds.StorageMode(), "storageMode must be keyring after MigrateToKeyring")

	// credentials.json must not contain any sensitive values
	data := readCredentialsJSON(t, fs)
	var jsonFile struct {
		Aura struct {
			Credentials []struct {
				ClientSecret string `json:"client-secret"`
				AccessToken  string `json:"access-token"`
			} `json:"credentials"`
		} `json:"aura"`
		Dbms struct {
			Credentials []struct {
				Password string `json:"password"`
			} `json:"credentials"`
		} `json:"dbms"`
		Embed struct {
			Credentials []struct {
				APIKey string `json:"api-key"`
			} `json:"credentials"`
		} `json:"embed"`
	}
	require.NoError(t, json.Unmarshal(data, &jsonFile))
	assert.Equal(t, "", jsonFile.Aura.Credentials[0].ClientSecret, "client-secret must be scrubbed from JSON")
	assert.Equal(t, "", jsonFile.Aura.Credentials[0].AccessToken, "access-token must be scrubbed from JSON")
	assert.Equal(t, "", jsonFile.Dbms.Credentials[0].Password, "password must be scrubbed from JSON")
	assert.Equal(t, "", jsonFile.Embed.Credentials[0].APIKey, "api-key must be scrubbed from JSON")
}

// TestMigrateToKeyring_EmptyRequired_Error verifies that MigrateToKeyring aborts
// with a named error when a required field is empty. Covers aura client-secret
// and dbms password.
func TestMigrateToKeyring_EmptyRequired_Error(t *testing.T) {
	tests := []struct {
		name      string
		credJSON  string
		wantInErr []string
	}{
		{
			name:      "aura empty client-secret",
			credJSON:  `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`,
			wantInErr: []string{"prod", "aura"},
		},
		{
			name:      "dbms empty password",
			credJSON:  `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			wantInErr: []string{"local", "dbms"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockKeyringProvider()
			creds, _ := newInsecureTestCredentials(t, mock, tc.credJSON)

			err := creds.MigrateToKeyring()
			require.Error(t, err)
			for _, s := range tc.wantInErr {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

// TestMigrateToKeyring_EmptyOptionalFieldsSkipped verifies that optional empty
// fields (AccessToken, APIKey) are silently skipped and no keyring entry is
// written for them.
func TestMigrateToKeyring_EmptyOptionalFieldsSkipped(t *testing.T) {
	mock := newMockKeyringProvider()
	creds, _ := newInsecureTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"","token-expiry":0}]},`+
			`"embed":{"credentials":[{"name":"openai","provider":"openai","model":"ada","base-url":"","dimensions":1536,"api-key":""}]}}`)

	require.NoError(t, creds.MigrateToKeyring())

	// access-token and api-key must NOT be in keyring
	_, err := mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "empty AccessToken must not be written to keyring")

	_, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("embed", "openai", "api-key"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "empty APIKey must not be written to keyring")
}

// TestMigrateToKeyring_PartialFailureRollback verifies that when a keyring.Set
// fails mid-migration, all previously written keyring entries are deleted.
func TestMigrateToKeyring_PartialFailureRollback(t *testing.T) {
	mock := newMockKeyringProvider()
	// Make the mock fail on the second Set call (dbms password) to simulate
	// a partial failure. We use a wrapper that errors after N writes.
	failing := &failAfterNProvider{inner: mock, failAfter: 1}
	credentials.SetKeyringProviderForTest(t, failing)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"","token-expiry":0}]},`+
		`"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"p4ss","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)
	require.NoError(t, err)
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)

	err = creds.MigrateToKeyring()
	require.Error(t, err, "should fail on second Set")

	// The first entry (aura/prod/client-secret) must have been rolled back
	_, getErr := mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	assert.ErrorIs(t, getErr, credentials.ErrNotFound, "rolled-back aura/prod/client-secret must not remain in keyring")
}

// failAfterNProvider wraps a mock provider and returns an error on Set after N
// successful writes, then delegates to the inner mock for Get/Delete.
type failAfterNProvider struct {
	inner     *mockKeyringProvider
	failAfter int
	setCount  int
}

func (f *failAfterNProvider) Get(service, user string) (string, error) {
	return f.inner.Get(service, user)
}

func (f *failAfterNProvider) Set(service, user, password string) error {
	if f.setCount >= f.failAfter {
		return errProviderFail
	}
	f.setCount++
	return f.inner.Set(service, user, password)
}

func (f *failAfterNProvider) Delete(service, user string) error {
	return f.inner.Delete(service, user)
}

// TestMigrateToKeyring_EmptyRequiredField_RollsBackPreviousEntries verifies
// that when a required field is empty (hard error path), entries written for
// earlier credentials are deleted (rolled back).
func TestMigrateToKeyring_EmptyRequiredField_RollsBackPreviousEntries(t *testing.T) {
	mock := newMockKeyringProvider()
	creds, _ := newInsecureTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"","token-expiry":0}]},`+
			`"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)

	err := creds.MigrateToKeyring()
	require.Error(t, err, "must fail on empty dbms password")

	// The previously-written aura/prod/client-secret must be rolled back
	_, getErr := mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	assert.ErrorIs(t, getErr, credentials.ErrNotFound, "rolled-back aura/prod/client-secret must not remain in keyring")
}

// TestMigrateToKeyring_KeyringUnavailable verifies that MigrateToKeyring returns
// a UsageError immediately when the keyring daemon is unreachable, before
// writing any keyring entries. The probe check runs even when no credentials
// exist, so both sub-cases are covered.
func TestMigrateToKeyring_KeyringUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		credJSON string
	}{
		{
			name:     "keyring unavailable with credentials present",
			credJSON: `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"s3cr3t","access-token":"","token-expiry":0}]}}`,
		},
		{
			name:     "keyring unavailable with no credentials",
			credJSON: `{"aura":{"credentials":[]}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// probeStubProvider (defined in keyring_test.go, same test package)
			// returns a non-ErrNotFound error for every Get, simulating an
			// unavailable keyring daemon.
			stub := &probeStubProvider{getErr: errors.New("keyring daemon unavailable")}
			credentials.SetKeyringProviderForTest(t, stub)

			fs, err := testfs.GetTestFs("{}", tc.credJSON)
			require.NoError(t, err)
			creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
			// Stay in insecure mode (default) so NewCredentials / load() does
			// not touch the keyring before MigrateToKeyring is called.

			migrateErr := creds.MigrateToKeyring()
			require.Error(t, migrateErr)
			assert.Contains(t, migrateErr.Error(), "keyring is unavailable")
			assert.NotContains(t, migrateErr.Error(), "config set credential-storage insecure")
		})
	}
}

// --- MigrateToInsecure tests ---

// TestMigrateToInsecure_Success verifies the happy path: all secrets are read
// from the keyring, written to credentials.json, and keyring entries are
// deleted afterwards.
func TestMigrateToInsecure_Success(t *testing.T) {
	mock := newMockKeyringProvider()
	// Pre-populate keyring as if MigrateToKeyring() had run previously
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"), "tok"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password"), "p4ss"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("embed", "openai", "api-key"), "sk-key"))

	// Credentials JSON has empty sensitive fields (scrubbed state)
	creds, fs := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]},`+
			`"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]},`+
			`"embed":{"credentials":[{"name":"openai","provider":"openai","model":"ada","base-url":"","dimensions":1536,"api-key":""}]}}`)

	require.NoError(t, creds.MigrateToInsecure())

	// In-memory sensitive fields must be populated
	assert.Equal(t, "s3cr3t", creds.Aura.Credentials[0].ClientSecret, "in-memory ClientSecret must be populated")
	assert.Equal(t, "tok", creds.Aura.Credentials[0].AccessToken, "in-memory AccessToken must be populated")
	assert.Equal(t, "p4ss", creds.Dbms.Credentials[0].Password, "in-memory Password must be populated")
	assert.Equal(t, "sk-key", creds.Embed.Credentials[0].APIKey, "in-memory APIKey must be populated")

	// credentials.json must contain the sensitive values
	data := readCredentialsJSON(t, fs)
	assert.Contains(t, string(data), "s3cr3t", "credentials.json must contain ClientSecret after migration")
	assert.Contains(t, string(data), "tok", "credentials.json must contain AccessToken after migration")
	assert.Contains(t, string(data), "p4ss", "credentials.json must contain Password after migration")
	assert.Contains(t, string(data), "sk-key", "credentials.json must contain APIKey after migration")

	// Keyring entries must have been deleted
	_, err := mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "keyring entry for aura/prod/client-secret must be deleted")
	_, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "keyring entry for aura/prod/access-token must be deleted")
	_, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "keyring entry for dbms/local/password must be deleted")
	_, err = mock.Get(credentials.ServiceName, credentials.KeyringKey("embed", "openai", "api-key"))
	assert.ErrorIs(t, err, credentials.ErrNotFound, "keyring entry for embed/openai/api-key must be deleted")
}

// TestMigrateToInsecure_RequiredFieldNotFound_ExternallyDeleted_SucceedsViaMemory
// verifies REQ-F-018: if the keyring entry for a required field is deleted
// externally AFTER SetStorageMode loaded it into memory, MigrateToInsecure
// still succeeds because the in-memory value is non-empty. The value is
// persisted to JSON by the final save() call.
func TestMigrateToInsecure_RequiredFieldNotFound_ExternallyDeleted_SucceedsViaMemory(t *testing.T) {
	tests := []struct {
		name       string
		keyringKey string
		seedValue  string
		credJSON   string
		wantInJSON string
	}{
		{
			name:       "aura externally-deleted client-secret uses in-memory value",
			keyringKey: credentials.KeyringKey("aura", "prod", "client-secret"),
			seedValue:  "s3cr3t",
			credJSON:   `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`,
			wantInJSON: "s3cr3t",
		},
		{
			name:       "dbms externally-deleted password uses in-memory value",
			keyringKey: credentials.KeyringKey("dbms", "local", "password"),
			seedValue:  "p4ss",
			credJSON:   `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			wantInJSON: "p4ss",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockKeyringProvider()
			// Seed so SetStorageMode loads the value into memory
			require.NoError(t, mock.Set(credentials.ServiceName, tc.keyringKey, tc.seedValue))
			creds, fs := newKeyringTestCredentials(t, mock, tc.credJSON)
			// Simulate external deletion after the process started
			require.NoError(t, mock.Delete(credentials.ServiceName, tc.keyringKey))

			// REQ-F-018: should succeed since in-memory value is non-empty
			require.NoError(t, creds.MigrateToInsecure(),
				"must succeed when in-memory value is available (REQ-F-018)")

			// Value must be persisted to JSON
			data := readCredentialsJSON(t, fs)
			assert.Contains(t, string(data), tc.wantInJSON,
				"credentials.json must contain the value after reverse migration")
		})
	}
}

// TestMigrateToInsecure_BothKeyringAndMemoryEmpty_Errors verifies that
// MigrateToInsecure returns a named error when both the keyring entry AND the
// in-memory value are absent for a required field — the hard error path that
// REQ-F-018 does NOT change.
func TestMigrateToInsecure_BothKeyringAndMemoryEmpty_Errors(t *testing.T) {
	tests := []struct {
		name      string
		credJSON  string
		wantInErr []string
	}{
		{
			name:      "aura missing client-secret both keyring and memory",
			credJSON:  `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`,
			wantInErr: []string{"prod", "client-secret"},
		},
		{
			name:      "dbms missing password both keyring and memory",
			credJSON:  `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			wantInErr: []string{"local", "password"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockKeyringProvider()
			// No entries seeded — both keyring and in-memory are empty.
			// Use insecure mode so that load() populates in-memory fields from
			// JSON (which has empty sensitive fields), then call MigrateToInsecure
			// directly. MigrateToInsecure always calls keyring.Get regardless of
			// storageMode; with empty keyring and empty in-memory it must error.
			credentials.SetKeyringProviderForTest(t, mock)
			fs, err := testfs.GetTestFs("{}", tc.credJSON)
			require.NoError(t, err)
			// Stay in insecure mode (default) — the in-memory sensitive fields
			// are empty (loaded from JSON which has empty values).
			creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)

			migrateErr := creds.MigrateToInsecure()
			require.Error(t, migrateErr, "must error when both keyring and in-memory are absent")
			for _, s := range tc.wantInErr {
				assert.Contains(t, migrateErr.Error(), s)
			}
		})
	}
}

// TestMigrateToInsecure_OptionalFieldNotFound_SilentlySkipped verifies that
// ErrNotFound for optional fields (AccessToken, APIKey) results in an empty
// value written to JSON. The test simulates the case where optional fields were
// never set in the keyring (e.g., the access token was absent during migration).
func TestMigrateToInsecure_OptionalFieldNotFound_SilentlySkipped(t *testing.T) {
	mock := newMockKeyringProvider()
	// Required ClientSecret is in keyring; optional access-token and api-key are absent
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	// No access-token or api-key set in keyring

	creds, fs := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]},`+
			`"embed":{"credentials":[{"name":"openai","provider":"openai","model":"ada","base-url":"","dimensions":1536,"api-key":""}]}}`)

	require.NoError(t, creds.MigrateToInsecure(), "ErrNotFound on optional fields must not abort migration")

	// In-memory: optional fields should be empty
	assert.Equal(t, "", creds.Aura.Credentials[0].AccessToken, "optional AccessToken must be empty when not in keyring")
	assert.Equal(t, "", creds.Embed.Credentials[0].APIKey, "optional APIKey must be empty when not in keyring")

	// credentials.json must have the required field but empty optional fields
	data := readCredentialsJSON(t, fs)
	assert.Contains(t, string(data), "s3cr3t", "credentials.json must contain ClientSecret")
}

// TestMigrateToInsecure_NonErrNotFound_IsHardError verifies that a non-ErrNotFound
// keyring error aborts migration even for optional fields. SetStorageMode is
// called with the real mock first (which succeeds), then the errorOnGetProvider
// is swapped in for the MigrateToInsecure call.
func TestMigrateToInsecure_NonErrNotFound_IsHardError(t *testing.T) {
	mock := newMockKeyringProvider()
	// Pre-populate the required client-secret so SetStorageMode succeeds
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))

	creds, _ := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`)

	// Now swap in a provider that fails with a non-ErrNotFound error on the access-token Get
	// to simulate a keyring error during MigrateToInsecure.
	failing := &errorOnGetProvider{inner: mock, failKey: credentials.KeyringKey("aura", "prod", "client-secret")}
	credentials.SetKeyringProviderForTest(t, failing)

	err := creds.MigrateToInsecure()
	require.Error(t, err, "non-ErrNotFound error must abort migration")
	assert.Contains(t, err.Error(), "aura/prod/client-secret")
}

// TestMigrateToInsecure_NoCredentials_Noop verifies that with no credentials
// MigrateToInsecure() succeeds and is a no-op.
func TestMigrateToInsecure_NoCredentials_Noop(t *testing.T) {
	mock := newMockKeyringProvider()

	creds, _ := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[]}}`)

	require.NoError(t, creds.MigrateToInsecure())
}

// TestMigrateToInsecure_RequiredFieldMissing_ZerosInMemoryFieldsAlreadySet
// verifies that when migration fails on a required field after an earlier
// in-memory field was set (e.g., AccessToken before Password), the already-set
// in-memory fields are zeroed.
func TestMigrateToInsecure_RequiredFieldMissing_ZerosInMemoryFieldsAlreadySet(t *testing.T) {
	mock := newMockKeyringProvider()
	// Provide all secrets so SetStorageMode succeeds.
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"), "tok"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password"), "p4ss"))

	creds, _ := newKeyringTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]},`+
			`"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)

	// Delete the dbms password after SetStorageMode so MigrateToInsecure sees ErrNotFound on required field.
	require.NoError(t, mock.Delete(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password")))
	// Also clear in-memory value to simulate the state after SetStorageMode
	// loaded it (the struct was populated from keyring during SetStorageMode).
	// We need to simulate that the aura fields were set in-memory during
	// MigrateToInsecure phase 1 before the dbms password fails.
	// Reset in-memory values to empty (as they would be in scrubbed JSON state).
	creds.Aura.Credentials[0].ClientSecret = ""
	creds.Aura.Credentials[0].AccessToken = ""
	creds.Dbms.Credentials[0].Password = ""

	// Also remove aura entries from keyring — we want MigrateToInsecure to
	// re-read from keyring. Re-populate aura entries but not dbms.
	_ = mock.Delete(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	_ = mock.Delete(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"), "s3cr3t"))
	require.NoError(t, mock.Set(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "access-token"), "tok"))

	err := creds.MigrateToInsecure()
	require.Error(t, err, "must fail on missing dbms password")
	assert.Contains(t, err.Error(), "local")

	// In-memory fields set during phase 1 must be zeroed
	assert.Equal(t, "", creds.Aura.Credentials[0].ClientSecret, "in-memory ClientSecret must be zeroed after failure")
	assert.Equal(t, "", creds.Aura.Credentials[0].AccessToken, "in-memory AccessToken must be zeroed after failure")
}

// errorOnGetProvider is a KeyringProvider that fails with a specific non-ErrNotFound
// error when getting a specific key. All other operations delegate to inner.
type errorOnGetProvider struct {
	inner   *mockKeyringProvider
	failKey string
}

var errGetFail = errors.New("simulated keyring get failure")

func (e *errorOnGetProvider) Get(service, user string) (string, error) {
	if user == e.failKey {
		return "", errGetFail
	}
	return e.inner.Get(service, user)
}

func (e *errorOnGetProvider) Set(service, user, password string) error {
	return e.inner.Set(service, user, password)
}

func (e *errorOnGetProvider) Delete(service, user string) error {
	return e.inner.Delete(service, user)
}
