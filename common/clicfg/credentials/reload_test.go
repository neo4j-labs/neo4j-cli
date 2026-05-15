// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type openErrFs struct {
	afero.Fs
}

func (fs openErrFs) Open(name string) (afero.File, error) {
	return nil, os.ErrPermission
}

// credentialsPath is the expected path of the credentials file within the
// testfs layout.
func credentialsPath() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
}

// writeCredsJSON marshals v to JSON and writes it to the credentials path in
// the supplied fs.
func writeCredsJSON(t *testing.T, fs afero.Fs, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, credentialsPath(), data, 0o600))
}

// freshToken returns a TokenExpiry 1 hour in the future (unix milliseconds).
func freshToken() int64 {
	return time.Now().Add(time.Hour).UnixMilli()
}

func TestReloadAuraCredential_ReturnsCredentialWhenPresent(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Dir(credentialsPath()), 0o700))

	expiry := freshToken()
	writeCredsJSON(t, fs, map[string]any{
		"aura": map[string]any{
			"default-credential": "my-cred",
			"credentials": []map[string]any{
				{
					"name":          "my-cred",
					"client-id":     "cid",
					"client-secret": "csec",
					"access-token":  "tok123",
					"token-expiry":  expiry,
				},
			},
		},
	})

	cred, ok := credentials.ReloadAuraCredential(fs, credentialsPath(), "my-cred")
	require.True(t, ok)
	require.NotNil(t, cred)
	assert.Equal(t, "my-cred", cred.Name)
	assert.Equal(t, "tok123", cred.AccessToken)
	assert.Equal(t, expiry, cred.TokenExpiry)
}

func TestReloadAuraCredential_ReturnsFalseWhenNameAbsent(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Dir(credentialsPath()), 0o700))

	writeCredsJSON(t, fs, map[string]any{
		"aura": map[string]any{
			"credentials": []map[string]any{
				{"name": "other", "client-id": "cid", "client-secret": "csec"},
			},
		},
	})

	cred, ok := credentials.ReloadAuraCredential(fs, credentialsPath(), "not-found")
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestReloadAuraCredential_ReturnsFalseWhenFileMissing(t *testing.T) {
	fs := afero.NewMemMapFs()

	cred, ok := credentials.ReloadAuraCredential(fs, credentialsPath(), "any")
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestReloadAuraCredential_ReturnsFalseWhenFileCorrupt(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Dir(credentialsPath()), 0o700))
	require.NoError(t, afero.WriteFile(fs, credentialsPath(), []byte("not-json"), 0o600))

	cred, ok := credentials.ReloadAuraCredential(fs, credentialsPath(), "any")
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestReloadAuraCredential_ReturnsFalseWhenAuraKeyAbsent(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Dir(credentialsPath()), 0o700))

	// Valid JSON but no "aura" key.
	writeCredsJSON(t, fs, map[string]any{"dbms": map[string]any{}})

	cred, ok := credentials.ReloadAuraCredential(fs, credentialsPath(), "any")
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestReloadAuraCredential_ReturnsFalseWhenReadFails(t *testing.T) {
	fs := openErrFs{Fs: afero.NewMemMapFs()}

	var (
		cred *credentials.AuraCredential
		ok   bool
	)
	require.NotPanics(t, func() {
		cred, ok = credentials.ReloadAuraCredential(fs, credentialsPath(), "any")
	})
	assert.False(t, ok)
	assert.Nil(t, cred)
}

func TestCredentials_FilePath_MatchesExpectedPath(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	assert.Equal(t, credentialsPath(), creds.FilePath())
}

func TestAuraCredentials_SyncCredential_UpdatesTokenWithoutDiskWrite(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"default-credential":"c1","credentials":[{"name":"c1","client-id":"cid","client-secret":"sec","access-token":"old","token-expiry":1}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)

	newExpiry := freshToken()
	fresh := &credentials.AuraCredential{
		Name:        "c1",
		AccessToken: "new-token",
		TokenExpiry: newExpiry,
	}

	// Capture file state before SyncCredential.
	before, err := afero.ReadFile(fs, credentialsPath())
	require.NoError(t, err)

	creds.Aura.SyncCredential(fresh)

	// In-memory state must reflect the new token.
	cred, err := creds.Aura.Get("c1")
	require.NoError(t, err)
	assert.Equal(t, "new-token", cred.AccessToken)
	assert.Equal(t, newExpiry, cred.TokenExpiry)

	// On-disk file must NOT have changed (no onUpdate call).
	after, err := afero.ReadFile(fs, credentialsPath())
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "SyncCredential must not write to disk")
}

func TestAuraCredentials_SyncCredential_NoopWhenNameAbsent(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)

	// Must not panic when the credential doesn't exist.
	creds.Aura.SyncCredential(&credentials.AuraCredential{
		Name:        "ghost",
		AccessToken: "tok",
		TokenExpiry: freshToken(),
	})
}
