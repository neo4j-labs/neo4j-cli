// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"bytes"
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

// emptyCredentialsJSON is a credentials.json with no stored credentials.
const emptyCredentialsJSON = `{"aura":{"credentials":[]},"dbms":{"credentials":[]},"embed":{"credentials":[]}}`

// newEnvTestCredentials builds a Credentials backed by an in-memory FS seeded
// with the given credentials JSON. It does NOT switch to env mode; the caller
// drives that via SetStorageMode after injecting the env seam.
func newEnvTestCredentials(t *testing.T, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	return creds, fs
}

// envFromMap returns a getenv-shaped lookup over a fixed map.
func envFromMap(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func readCredentialsFile(t *testing.T, fs afero.Fs) string {
	t.Helper()
	path := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	data, err := afero.ReadFile(fs, path)
	require.NoError(t, err)
	return string(data)
}

func TestLoadFromEnv_Synthesis(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		// assert inspects the synthesized credentials and the captured warning.
		assert func(t *testing.T, creds *credentials.Credentials, warn string)
	}{
		{
			name: "aura synthesizes when both id and secret set",
			env: map[string]string{
				"NEO4J_AURA_CLIENT_ID":     "the-id",
				"NEO4J_AURA_CLIENT_SECRET": "the-secret",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				assert.Empty(t, warn)
				cred, err := creds.Aura.GetDefault()
				require.NoError(t, err)
				assert.Equal(t, "env", cred.Name)
				assert.Equal(t, "the-id", cred.ClientId)
				assert.Equal(t, "the-secret", cred.ClientSecret)
				assert.Empty(t, cred.AccessToken, "access token left empty for first OAuth fetch")
				assert.Equal(t, "env", creds.Aura.DefaultCredential)
			},
		},
		{
			name: "aura with only client id set warns and does not synthesize",
			env: map[string]string{
				"NEO4J_AURA_CLIENT_ID": "the-id",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				assert.Contains(t, warn, "NEO4J_AURA_CLIENT_ID")
				assert.Contains(t, warn, "NEO4J_AURA_CLIENT_SECRET")
				assert.Empty(t, creds.Aura.Credentials)
				assert.Empty(t, creds.Aura.DefaultCredential)
			},
		},
		{
			name: "aura with only client secret set warns and does not synthesize",
			env: map[string]string{
				"NEO4J_AURA_CLIENT_SECRET": "the-secret",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				assert.Contains(t, warn, "Aura credential not synthesized")
				assert.Empty(t, creds.Aura.Credentials)
			},
		},
		{
			name: "dbms synthesizes on password populating uri/username/database",
			env: map[string]string{
				"NEO4J_PASSWORD": "pw",
				"NEO4J_URI":      "bolt://host:7687",
				"NEO4J_USERNAME": "neo4j",
				"NEO4J_DATABASE": "mydb",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				cred, err := creds.Dbms.GetDefault()
				require.NoError(t, err)
				require.NotNil(t, cred)
				assert.Equal(t, "env", cred.Name)
				assert.Equal(t, "pw", cred.Password)
				assert.Equal(t, "bolt://host:7687", cred.URI)
				assert.Equal(t, "neo4j", cred.Username)
				assert.Equal(t, "mydb", cred.DatabaseName)
				assert.Equal(t, "env", creds.Dbms.DefaultCredential)
			},
		},
		{
			name: "dbms does not synthesize without password",
			env: map[string]string{
				"NEO4J_URI":      "bolt://host:7687",
				"NEO4J_USERNAME": "neo4j",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				assert.Empty(t, creds.Dbms.Credentials)
			},
		},
		{
			name: "embed synthesizes only on provider, populating fields",
			env: map[string]string{
				"NEO4J_EMBED_PROVIDER":   "openai",
				"NEO4J_EMBED_MODEL":      "text-embedding-3-small",
				"NEO4J_EMBED_BASE_URL":   "https://api.example.com",
				"NEO4J_EMBED_DIMENSIONS": "1536",
				"NEO4J_EMBED_API_KEY":    "sk-xyz",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				cred, err := creds.Embed.GetDefault()
				require.NoError(t, err)
				require.NotNil(t, cred)
				assert.Equal(t, "env", cred.Name)
				assert.Equal(t, "openai", cred.Provider)
				assert.Equal(t, "text-embedding-3-small", cred.Model)
				assert.Equal(t, "https://api.example.com", cred.BaseURL)
				assert.Equal(t, 1536, cred.Dimensions)
				assert.Equal(t, "sk-xyz", cred.APIKey)
				assert.Equal(t, "env", creds.Embed.DefaultCredential)
			},
		},
		{
			name: "embed does not synthesize on api key alone",
			env: map[string]string{
				"NEO4J_EMBED_API_KEY": "sk-xyz",
				"NEO4J_EMBED_MODEL":   "text-embedding-3-small",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				assert.Empty(t, creds.Embed.Credentials)
			},
		},
		{
			name: "all three types synthesize together",
			env: map[string]string{
				"NEO4J_AURA_CLIENT_ID":     "id",
				"NEO4J_AURA_CLIENT_SECRET": "secret",
				"NEO4J_PASSWORD":           "pw",
				"NEO4J_EMBED_PROVIDER":     "openai",
			},
			assert: func(t *testing.T, creds *credentials.Credentials, warn string) {
				assert.Empty(t, warn)
				assert.Len(t, creds.Aura.Credentials, 1)
				assert.Len(t, creds.Dbms.Credentials, 1)
				assert.Len(t, creds.Embed.Credentials, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			credentials.SetGetenvForTest(t, envFromMap(tc.env))
			creds, _ := newEnvTestCredentials(t, emptyCredentialsJSON)

			var warn bytes.Buffer
			creds.SetStorageMode(credentials.StorageModeEnv, &warn)

			tc.assert(t, creds, warn.String())
		})
	}
}

// TestLoadFromEnv_OverlaysExistingEnvCredential verifies that synthesis reuses a
// pre-existing in-memory "env" credential rather than appending a duplicate.
func TestLoadFromEnv_OverlaysExistingEnvCredential(t *testing.T) {
	const seeded = `{"aura":{"default-credential":"env","credentials":[{"name":"env","client-id":"old","client-secret":"old"}]},"dbms":{"credentials":[]},"embed":{"credentials":[]}}`

	credentials.SetGetenvForTest(t, envFromMap(map[string]string{
		"NEO4J_AURA_CLIENT_ID":     "new-id",
		"NEO4J_AURA_CLIENT_SECRET": "new-secret",
	}))
	creds, _ := newEnvTestCredentials(t, seeded)

	var warn bytes.Buffer
	creds.SetStorageMode(credentials.StorageModeEnv, &warn)

	require.Len(t, creds.Aura.Credentials, 1, "must overlay, not duplicate the env credential")
	cred := creds.Aura.Credentials[0]
	assert.Equal(t, "env", cred.Name)
	assert.Equal(t, "new-id", cred.ClientId)
	assert.Equal(t, "new-secret", cred.ClientSecret)
}

// TestEnvMode_SaveIsNoOp verifies that mutating credentials in env mode writes
// nothing to disk (the MemMapFs credentials.json is unchanged) and never calls
// the keyring Set.
func TestEnvMode_SaveIsNoOp(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)
	credentials.SetGetenvForTest(t, envFromMap(map[string]string{
		"NEO4J_AURA_CLIENT_ID":     "id",
		"NEO4J_AURA_CLIENT_SECRET": "secret",
		"NEO4J_PASSWORD":           "pw",
		"NEO4J_EMBED_PROVIDER":     "openai",
	}))

	creds, fs := newEnvTestCredentials(t, emptyCredentialsJSON)
	before := readCredentialsFile(t, fs)

	var warn bytes.Buffer
	creds.SetStorageMode(credentials.StorageModeEnv, &warn)

	// Mutate each credential type via the public surface, which triggers onUpdate
	// → save(). In env mode save() must be a no-op.
	require.NoError(t, creds.Aura.SetDefault("env"))
	require.NoError(t, creds.Dbms.SetDefault("env"))
	require.NoError(t, creds.Embed.SetDefault("env"))

	after := readCredentialsFile(t, fs)
	assert.Equal(t, before, after, "credentials.json must be untouched in env mode")
	assert.Zero(t, mock.setCalls, "no keyring Set calls may occur in env mode")
}

// TestEnvMode_TokenRefreshStaysInMemory verifies that an Aura token refresh in
// env mode updates the in-memory credential but persists nothing.
func TestEnvMode_TokenRefreshStaysInMemory(t *testing.T) {
	mock := newMockKeyringProvider()
	credentials.SetKeyringProviderForTest(t, mock)
	credentials.SetGetenvForTest(t, envFromMap(map[string]string{
		"NEO4J_AURA_CLIENT_ID":     "id",
		"NEO4J_AURA_CLIENT_SECRET": "secret",
	}))

	creds, fs := newEnvTestCredentials(t, emptyCredentialsJSON)
	before := readCredentialsFile(t, fs)

	var warn bytes.Buffer
	creds.SetStorageMode(credentials.StorageModeEnv, &warn)

	cred, err := creds.Aura.GetDefault()
	require.NoError(t, err)

	updated, err := creds.Aura.UpdateAccessToken(cred, "tok-123", 3600)
	require.NoError(t, err)
	assert.Equal(t, "tok-123", updated.AccessToken, "in-memory token must be updated")
	assert.True(t, updated.HasValidAccessToken())

	after := readCredentialsFile(t, fs)
	assert.Equal(t, before, after, "token refresh must not write to disk in env mode")
	assert.Zero(t, mock.setCalls, "token refresh must not write to keyring in env mode")
}

// TestAdd_ReservedEnvName verifies that the reserved "env" credential name is
// rejected on the Add path for all three credential types.
func TestAdd_ReservedEnvName(t *testing.T) {
	creds, _ := newEnvTestCredentials(t, emptyCredentialsJSON)

	t.Run("aura", func(t *testing.T) {
		err := creds.Aura.Add("env", "id", "secret")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved")
		assert.Empty(t, creds.Aura.Credentials)
	})

	t.Run("dbms", func(t *testing.T) {
		err := creds.Dbms.Add("env", "neo4j", "pw", "neo4j", "bolt://host:7687")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved")
		assert.Empty(t, creds.Dbms.Credentials)
	})

	t.Run("embed", func(t *testing.T) {
		err := creds.Embed.Add("env", "openai", "model", "", "key", 0, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved")
		assert.Empty(t, creds.Embed.Credentials)
	})
}

// TestWarnIfEnvMode verifies the warning fires only in env mode.
func TestWarnIfEnvMode(t *testing.T) {
	t.Run("env mode warns", func(t *testing.T) {
		credentials.SetGetenvForTest(t, envFromMap(nil))
		creds, _ := newEnvTestCredentials(t, emptyCredentialsJSON)
		creds.SetStorageMode(credentials.StorageModeEnv, io.Discard)

		var w bytes.Buffer
		creds.WarnIfEnvMode(&w)
		assert.Contains(t, w.String(), "not persisted")
	})

	t.Run("insecure mode is silent", func(t *testing.T) {
		creds, _ := newEnvTestCredentials(t, emptyCredentialsJSON)
		var w bytes.Buffer
		creds.WarnIfEnvMode(&w)
		assert.Empty(t, w.String())
	})
}
