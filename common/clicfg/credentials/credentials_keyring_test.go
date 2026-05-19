// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"encoding/json"
	"errors"
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

// TestMigrateToKeyring_EmptyRequiredAuraClientSecret aborts with a named error
// when an Aura credential has an empty ClientSecret.
func TestMigrateToKeyring_EmptyRequiredAuraClientSecret(t *testing.T) {
	mock := newMockKeyringProvider()
	creds, _ := newInsecureTestCredentials(t, mock,
		`{"aura":{"credentials":[{"name":"prod","client-id":"id1","client-secret":"","access-token":"","token-expiry":0}]}}`)

	err := creds.MigrateToKeyring()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prod", "error must name the credential")
	assert.Contains(t, err.Error(), "aura", "error must identify the credential type")
}

// TestMigrateToKeyring_EmptyRequiredDbmsPassword aborts with a named error
// when a Dbms credential has an empty Password.
func TestMigrateToKeyring_EmptyRequiredDbmsPassword(t *testing.T) {
	mock := newMockKeyringProvider()
	creds, _ := newInsecureTestCredentials(t, mock,
		`{"dbms":{"credentials":[{"name":"local","username":"neo4j","password":"","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)

	err := creds.MigrateToKeyring()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local", "error must name the credential")
	assert.Contains(t, err.Error(), "dbms", "error must identify the credential type")
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
