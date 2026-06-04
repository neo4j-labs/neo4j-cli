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

func newTestEmbedCredentials(t *testing.T, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	cfg := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	return cfg, fs
}

func TestEmbedCredentials_Add(t *testing.T) {
	tests := []struct {
		name           string
		initialJSON    string
		addName        string
		addProvider    string
		addModel       string
		addBaseURL     string
		addAPIKey      string
		addDimensions  int
		addVertexProj  string
		addVertexLoc   string
		wantErr        string
		wantDefault    string
		wantCredCount  int
		wantVertexProj string
		wantVertexLoc  string
	}{
		{
			name:          "add first credential sets it as default",
			initialJSON:   `{"aura":{"credentials":[]}}`,
			addName:       "openai-default",
			addProvider:   "openai",
			addModel:      "text-embedding-3-small",
			addBaseURL:    "https://api.openai.com/v1",
			addAPIKey:     "sk-secret",
			addDimensions: 1536,
			wantErr:       "",
			wantDefault:   "openai-default",
			wantCredCount: 1,
		},
		{
			name:          "add second credential does not change default",
			initialJSON:   `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`,
			addName:       "second",
			addProvider:   "ollama",
			addModel:      "nomic-embed-text",
			addBaseURL:    "http://localhost:11434",
			addAPIKey:     "",
			addDimensions: 0,
			wantErr:       "",
			wantDefault:   "first",
			wantCredCount: 2,
		},
		{
			name:          "duplicate name returns error",
			initialJSON:   `{"aura":{"credentials":[]},"embed":{"default-credential":"local","credentials":[{"name":"local","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`,
			addName:       "local",
			addProvider:   "openai",
			addModel:      "m",
			addBaseURL:    "u",
			addAPIKey:     "k",
			addDimensions: 0,
			wantErr:       "already have credential with name local",
			wantDefault:   "local",
			wantCredCount: 1,
		},
		{
			name:           "add vertex credential stores project and location",
			initialJSON:    `{"aura":{"credentials":[]}}`,
			addName:        "vertex-default",
			addProvider:    "vertex",
			addModel:       "text-embedding-005",
			addBaseURL:     "",
			addAPIKey:      "",
			addDimensions:  0,
			addVertexProj:  "my-gcp-project",
			addVertexLoc:   "us-central1",
			wantErr:        "",
			wantDefault:    "vertex-default",
			wantCredCount:  1,
			wantVertexProj: "my-gcp-project",
			wantVertexLoc:  "us-central1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestEmbedCredentials(t, tc.initialJSON)
			err := creds.Embed.Add(tc.addName, tc.addProvider, tc.addModel, tc.addBaseURL, tc.addAPIKey, tc.addDimensions, tc.addVertexProj, tc.addVertexLoc)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Embed.DefaultCredential)
			assert.Len(t, creds.Embed.Credentials, tc.wantCredCount)
			if tc.wantVertexProj != "" || tc.wantVertexLoc != "" {
				cred, getErr := creds.Embed.Get(tc.addName)
				require.NoError(t, getErr)
				assert.Equal(t, tc.wantVertexProj, cred.VertexProject)
				assert.Equal(t, tc.wantVertexLoc, cred.VertexLocation)
			}
		})
	}
}

func TestEmbedCredentials_Remove(t *testing.T) {
	tests := []struct {
		name          string
		initialJSON   string
		removeName    string
		wantErr       string
		wantDefault   string
		wantCredCount int
	}{
		{
			name:          "remove existing non-default credential",
			initialJSON:   `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"},{"name":"second","provider":"ollama","model":"m","base-url":"u","dimensions":0,"api-key":""}]}}`,
			removeName:    "second",
			wantErr:       "",
			wantDefault:   "first",
			wantCredCount: 1,
		},
		{
			name:          "remove default credential clears default",
			initialJSON:   `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"},{"name":"second","provider":"ollama","model":"m","base-url":"u","dimensions":0,"api-key":""}]}}`,
			removeName:    "first",
			wantErr:       "",
			wantDefault:   "",
			wantCredCount: 1,
		},
		{
			name:          "remove unknown credential returns error",
			initialJSON:   `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`,
			removeName:    "nonexistent",
			wantErr:       "could not find credential with name nonexistent to remove",
			wantDefault:   "first",
			wantCredCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestEmbedCredentials(t, tc.initialJSON)
			err := creds.Embed.Remove(tc.removeName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Embed.DefaultCredential)
			assert.Len(t, creds.Embed.Credentials, tc.wantCredCount)
		})
	}
}

func TestEmbedCredentials_SetDefault(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		setName     string
		wantErr     string
		wantDefault string
	}{
		{
			name:        "set default to existing credential",
			initialJSON: `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"},{"name":"second","provider":"ollama","model":"m","base-url":"u","dimensions":0,"api-key":""}]}}`,
			setName:     "second",
			wantErr:     "",
			wantDefault: "second",
		},
		{
			name:        "set default to unknown credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`,
			setName:     "nonexistent",
			wantErr:     "could not find credential with name nonexistent",
			wantDefault: "first",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestEmbedCredentials(t, tc.initialJSON)
			err := creds.Embed.SetDefault(tc.setName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Embed.DefaultCredential)
		})
	}
}

func TestEmbedCredentials_GetDefault(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		wantNil     bool
		wantName    string
	}{
		{
			name:        "returns nil when no default set",
			initialJSON: `{"aura":{"credentials":[]},"embed":{"credentials":[]}}`,
			wantNil:     true,
		},
		{
			name:        "returns default credential when set",
			initialJSON: `{"aura":{"credentials":[]},"embed":{"default-credential":"local","credentials":[{"name":"local","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`,
			wantNil:     false,
			wantName:    "local",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestEmbedCredentials(t, tc.initialJSON)
			cred, err := creds.Embed.GetDefault()
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, cred)
			} else {
				require.NotNil(t, cred)
				assert.Equal(t, tc.wantName, cred.Name)
			}
		})
	}
}

func TestEmbedCredentials_Get(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		getName     string
		wantErr     string
		wantName    string
	}{
		{
			name:        "get existing credential returns it",
			initialJSON: `{"aura":{"credentials":[]},"embed":{"default-credential":"local","credentials":[{"name":"local","provider":"openai","model":"text-embedding-3-small","base-url":"https://api.openai.com/v1","dimensions":1536,"api-key":"sk-secret"}]}}`,
			getName:     "local",
			wantErr:     "",
			wantName:    "local",
		},
		{
			name:        "get unknown credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"embed":{"default-credential":"local","credentials":[{"name":"local","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`,
			getName:     "nonexistent",
			wantErr:     "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestEmbedCredentials(t, tc.initialJSON)
			cred, err := creds.Embed.Get(tc.getName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Nil(t, cred)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cred)
				assert.Equal(t, tc.wantName, cred.Name)
			}
		})
	}
}

func TestEmbedCredentials_List(t *testing.T) {
	tests := []struct {
		name          string
		initialJSON   string
		wantCredCount int
	}{
		{
			name:          "empty list returns empty slice",
			initialJSON:   `{"aura":{"credentials":[]}}`,
			wantCredCount: 0,
		},
		{
			name:          "list returns all credentials",
			initialJSON:   `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"},{"name":"second","provider":"ollama","model":"m","base-url":"u","dimensions":0,"api-key":""}]}}`,
			wantCredCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestEmbedCredentials(t, tc.initialJSON)
			list := creds.Embed.List()
			assert.Len(t, list, tc.wantCredCount)
		})
	}
}

func TestEmbedCredentials_Persist(t *testing.T) {
	// Verify that mutations persist to the file via onUpdate callback
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Embed.Add("openai-default", "openai", "text-embedding-3-small", "https://api.openai.com/v1", "sk-secret", 1536, "", ""))

	// Reload credentials from the same FS to verify persistence
	creds2 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.Len(t, creds2.Embed.Credentials, 1)
	assert.Equal(t, "openai-default", creds2.Embed.Credentials[0].Name)
	assert.Equal(t, "openai-default", creds2.Embed.DefaultCredential)
	// API key must be preserved on disk
	assert.Equal(t, "sk-secret", creds2.Embed.Credentials[0].APIKey)
	assert.Equal(t, 1536, creds2.Embed.Credentials[0].Dimensions)
}

func TestEmbedCredentials_PersistAfterLoad(t *testing.T) {
	// Verify that onUpdate is rewired after JSON unmarshal: mutations after a reload should still persist.
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Embed.Add("second", "ollama", "nomic-embed-text", "http://localhost:11434", "", 0, "", ""))

	creds2 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.Len(t, creds2.Embed.Credentials, 2)
}

func TestPrintableEmbedCredentials_AsArray(t *testing.T) {
	creds, _ := newTestEmbedCredentials(t, `{"aura":{"credentials":[]},"embed":{"default-credential":"first","credentials":[{"name":"first","provider":"openai","model":"text-embedding-3-small","base-url":"https://api.openai.com/v1","dimensions":1536,"api-key":"sk-secret"},{"name":"second","provider":"ollama","model":"nomic-embed-text","base-url":"http://localhost:11434","dimensions":0,"api-key":""}]}}`)

	printable := creds.Embed.Printable()
	rows := printable.AsArray()

	require.Len(t, rows, 2)

	// First credential is the default
	assert.Equal(t, "first", rows[0]["name"])
	assert.Equal(t, "openai", rows[0]["provider"])
	assert.Equal(t, "text-embedding-3-small", rows[0]["model"])
	assert.Equal(t, "https://api.openai.com/v1", rows[0]["base_url"])
	assert.Equal(t, 1536, rows[0]["dimensions"])
	assert.Equal(t, true, rows[0]["default"])
	// api-key must not appear in output
	_, hasAPIKey := rows[0]["api-key"]
	assert.False(t, hasAPIKey, "api-key must not appear in AsArray output")
	// vertex_* keys must be omitted when empty
	_, hasVertexProj := rows[0]["vertex_project"]
	assert.False(t, hasVertexProj, "vertex_project must be omitted when empty")
	_, hasVertexLoc := rows[0]["vertex_location"]
	assert.False(t, hasVertexLoc, "vertex_location must be omitted when empty")

	// Second credential is not the default
	assert.Equal(t, "second", rows[1]["name"])
	assert.Equal(t, false, rows[1]["default"])
}

func TestPrintableEmbedCredentials_AsArray_VertexFieldsIncluded(t *testing.T) {
	creds, _ := newTestEmbedCredentials(t, `{"aura":{"credentials":[]},"embed":{"default-credential":"vx","credentials":[{"name":"vx","provider":"vertex","model":"text-embedding-005","base-url":"","dimensions":0,"api-key":"","vertex-project":"my-gcp-project","vertex-location":"us-central1"}]}}`)

	printable := creds.Embed.Printable()
	rows := printable.AsArray()

	require.Len(t, rows, 1)
	assert.Equal(t, "my-gcp-project", rows[0]["vertex_project"])
	assert.Equal(t, "us-central1", rows[0]["vertex_location"])
}

func TestPrintableEmbedCredentials_MarshalJSON(t *testing.T) {
	creds, _ := newTestEmbedCredentials(t, `{"aura":{"credentials":[]},"embed":{"default-credential":"local","credentials":[{"name":"local","provider":"openai","model":"text-embedding-3-small","base-url":"https://api.openai.com/v1","dimensions":1536,"api-key":"sk-secret"}]}}`)

	printable := creds.Embed.Printable()
	data, err := json.Marshal(printable)
	require.NoError(t, err)

	var result []map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	require.Len(t, result, 1)

	assert.Equal(t, "local", result[0]["name"])
	assert.Equal(t, "openai", result[0]["provider"])
	// api-key must not appear in JSON output
	_, hasAPIKey := result[0]["api-key"]
	assert.False(t, hasAPIKey, "api-key must not appear in JSON output")
}

func TestEmbedCredentials_OnDiskRoundTripPreservesAPIKey(t *testing.T) {
	// Confirm that the credentials.json file on disk preserves api-key
	// even though Printable / MarshalJSON omit it.
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Embed.Add("openai-default", "openai", "text-embedding-3-small", "https://api.openai.com/v1", "sk-secret", 1536, "", ""))

	// Read credentials.json directly off the test FS and verify api-key is present.
	data, err := afero.ReadFile(fs, filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	embed, ok := raw["embed"].(map[string]any)
	require.True(t, ok, "embed key must be present on disk")

	credsArr, ok := embed["credentials"].([]any)
	require.True(t, ok)
	require.Len(t, credsArr, 1)

	first, ok := credsArr[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "sk-secret", first["api-key"], "api-key must be persisted on disk")
	assert.Equal(t, float64(1536), first["dimensions"])
}
