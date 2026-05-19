// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newKeyringTestCredentials creates a Credentials instance in keyring mode
// backed by a mock keyring provider. The caller can pre-populate the mock
// before calling this helper.
func newKeyringTestCredentials(t *testing.T, mock *mockKeyringProvider, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	credentials.SetKeyringProviderForTest(t, mock)
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring))
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
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeInsecure))

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
// JSON, SetStorageMode returns a UsageError (REQ-NF-004).
func TestSetStorageMode_KeyringMode_MissingRequired_Error(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	// JSON has no client-secret and keyring has nothing → should error
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	err = creds.SetStorageMode(credentials.StorageModeKeyring)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prod", "error must name the credential")
	assert.Contains(t, err.Error(), "aura client-secret", "error must name the missing field")
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
	err = creds.SetStorageMode(credentials.StorageModeKeyring)
	require.NoError(t, err, "pre-migration fallback must not error")
	assert.Equal(t, "json-secret", creds.Aura.Credentials[0].ClientSecret,
		"JSON value must be used when keyring has no entry")
}

// TestSave_KeyringMode_ZerosSensitiveFields verifies that in keyring mode
// save() writes empty strings for sensitive fields in credentials.json.
func TestSave_KeyringMode_ZerosSensitiveFields(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring))

	// Add a credential in keyring mode — this triggers save() via onUpdate
	require.NoError(t, creds.Aura.Add("prod", "id1", "s3cr3t"))

	// credentials.json must NOT contain the secret
	data := readCredentialsJSON(t, fs)
	assert.NotContains(t, string(data), "s3cr3t", "credentials.json must not contain sensitive field in keyring mode")

	// The keyring must hold the secret
	val, err := mock.Get(credentials.ServiceName, credentials.KeyringKey("aura", "prod", "client-secret"))
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", val)
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
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring))
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

// TestSave_KeyringMode_DbmsPassword verifies that dbms Password is stored in
// the keyring and zeroed in JSON.
func TestSave_KeyringMode_DbmsPassword(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring))
	require.NoError(t, creds.Dbms.Add("local", "neo4j", "p4ssword", "neo4j", "bolt://localhost:7687"))

	data := readCredentialsJSON(t, fs)
	assert.NotContains(t, string(data), "p4ssword")

	val, err := mock.Get(credentials.ServiceName, credentials.KeyringKey("dbms", "local", "password"))
	require.NoError(t, err)
	assert.Equal(t, "p4ssword", val)
}

// TestSave_KeyringMode_EmbedAPIKey verifies that embed APIKey is stored in
// the keyring and zeroed in JSON.
func TestSave_KeyringMode_EmbedAPIKey(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.SetStorageMode(credentials.StorageModeKeyring))
	require.NoError(t, creds.Embed.Add("openai", "openai", "text-embedding-ada-002", "", "sk-key123", 1536))

	data := readCredentialsJSON(t, fs)
	assert.NotContains(t, string(data), "sk-key123")

	val, err := mock.Get(credentials.ServiceName, credentials.KeyringKey("embed", "openai", "api-key"))
	require.NoError(t, err)
	assert.Equal(t, "sk-key123", val)
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
	require.NoError(t, creds1.SetStorageMode(credentials.StorageModeKeyring))
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
	require.NoError(t, creds2.SetStorageMode(credentials.StorageModeKeyring))
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

// TestSetStorageMode_KeyringMode_DbmsMissingPassword_Error verifies REQ-NF-004
// for a dbms credential missing its password from both keyring and JSON.
func TestSetStorageMode_KeyringMode_DbmsMissingPassword_Error(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)

	// Password is empty in JSON and absent from keyring
	fs, err := testfs.GetTestFs("{}", `{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	err = creds.SetStorageMode(credentials.StorageModeKeyring)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local")
	assert.Contains(t, err.Error(), "dbms password")
}
