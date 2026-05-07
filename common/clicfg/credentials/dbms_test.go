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

func newTestDbmsCredentials(t *testing.T, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	cfg := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	return cfg, fs
}

func TestDbmsCredentials_Add(t *testing.T) {
	tests := []struct {
		name          string
		initialJSON   string
		addName       string
		addUsername   string
		addPassword   string
		addDBName     string
		addURI        string
		wantErr       string
		wantDefault   string
		wantCredCount int
	}{
		{
			name:          "add first credential sets it as default",
			initialJSON:   `{"aura":{"credentials":[]}}`,
			addName:       "local",
			addUsername:   "neo4j",
			addPassword:   "secret",
			addDBName:     "neo4j",
			addURI:        "bolt://localhost:7687",
			wantErr:       "",
			wantDefault:   "local",
			wantCredCount: 1,
		},
		{
			name:          "add second credential does not change default",
			initialJSON:   `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			addName:       "second",
			addUsername:   "u2",
			addPassword:   "p2",
			addDBName:     "test",
			addURI:        "bolt://localhost:7688",
			wantErr:       "",
			wantDefault:   "first",
			wantCredCount: 2,
		},
		{
			name:          "duplicate name returns error",
			initialJSON:   `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			addName:       "local",
			addUsername:   "u",
			addPassword:   "p",
			addDBName:     "neo4j",
			addURI:        "bolt://localhost:7687",
			wantErr:       "already have credential with name local",
			wantDefault:   "local",
			wantCredCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			err := creds.Dbms.Add(tc.addName, tc.addUsername, tc.addPassword, tc.addDBName, tc.addURI)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Dbms.DefaultCredential)
			assert.Len(t, creds.Dbms.Credentials, tc.wantCredCount)
		})
	}
}

func TestDbmsCredentials_Remove(t *testing.T) {
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
			initialJSON:   `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688"}]}}`,
			removeName:    "second",
			wantErr:       "",
			wantDefault:   "first",
			wantCredCount: 1,
		},
		{
			name:          "remove default credential clears default",
			initialJSON:   `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688"}]}}`,
			removeName:    "first",
			wantErr:       "",
			wantDefault:   "",
			wantCredCount: 1,
		},
		{
			name:          "remove unknown credential returns error",
			initialJSON:   `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			removeName:    "nonexistent",
			wantErr:       "could not find credential with name nonexistent to remove",
			wantDefault:   "first",
			wantCredCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			err := creds.Dbms.Remove(tc.removeName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Dbms.DefaultCredential)
			assert.Len(t, creds.Dbms.Credentials, tc.wantCredCount)
		})
	}
}

func TestDbmsCredentials_SetDefault(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		setName     string
		wantErr     string
		wantDefault string
	}{
		{
			name:        "set default to existing credential",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688"}]}}`,
			setName:     "second",
			wantErr:     "",
			wantDefault: "second",
		},
		{
			name:        "set default to unknown credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			setName:     "nonexistent",
			wantErr:     "could not find credential with name nonexistent",
			wantDefault: "first",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			err := creds.Dbms.SetDefault(tc.setName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Dbms.DefaultCredential)
		})
	}
}

func TestDbmsCredentials_GetDefault(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		wantNil     bool
		wantName    string
	}{
		{
			name:        "returns nil when no default set",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"credentials":[]}}`,
			wantNil:     true,
		},
		{
			name:        "returns default credential when set",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			wantNil:     false,
			wantName:    "local",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			cred, err := creds.Dbms.GetDefault()
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

func TestDbmsCredentials_Get(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		getName     string
		wantErr     string
		wantName    string
	}{
		{
			name:        "get existing credential returns it",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"user1","password":"pass1","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			getName:     "local",
			wantErr:     "",
			wantName:    "local",
		},
		{
			name:        "get unknown credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			getName:     "nonexistent",
			wantErr:     "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			cred, err := creds.Dbms.Get(tc.getName)
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

func TestDbmsCredentials_List(t *testing.T) {
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
			initialJSON:   `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688"}]}}`,
			wantCredCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			list := creds.Dbms.List()
			assert.Len(t, list, tc.wantCredCount)
		})
	}
}

func TestDbmsCredentials_Persist(t *testing.T) {
	// Verify that mutations persist to the file via onUpdate callback
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Dbms.Add("local", "neo4j", "secret", "neo4j", "bolt://localhost:7687"))

	// Reload credentials from the same FS to verify persistence
	creds2 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.Len(t, creds2.Dbms.Credentials, 1)
	assert.Equal(t, "local", creds2.Dbms.Credentials[0].Name)
	assert.Equal(t, "local", creds2.Dbms.DefaultCredential)
}

func TestPrintableDbmsCredentials_AsArray(t *testing.T) {
	creds, _ := newTestDbmsCredentials(t, `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"user1","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"user2","password":"hidden","database-name":"test","uri":"bolt://localhost:7688"}]}}`)

	printable := creds.Dbms.Printable()
	rows := printable.AsArray()

	require.Len(t, rows, 2)

	// First credential is the default
	assert.Equal(t, "first", rows[0]["name"])
	assert.Equal(t, "user1", rows[0]["username"])
	assert.Equal(t, "neo4j", rows[0]["database-name"])
	assert.Equal(t, "bolt://localhost:7687", rows[0]["uri"])
	assert.Equal(t, true, rows[0]["default"])
	// Insecure must not appear in output
	_, hasInsecure := rows[0]["insecure"]
	assert.False(t, hasInsecure, "insecure must not appear in AsArray output")
	// Password must not appear in output
	_, hasPassword := rows[0]["password"]
	assert.False(t, hasPassword, "password must not appear in AsArray output")

	// Second credential is not the default
	assert.Equal(t, "second", rows[1]["name"])
	assert.Equal(t, false, rows[1]["default"])
}

func TestPrintableDbmsCredentials_MarshalJSON(t *testing.T) {
	creds, _ := newTestDbmsCredentials(t, `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"user1","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)

	printable := creds.Dbms.Printable()
	data, err := json.Marshal(printable)
	require.NoError(t, err)

	var result []map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	require.Len(t, result, 1)

	assert.Equal(t, "local", result[0]["name"])
	assert.Equal(t, "user1", result[0]["username"])
	// Password must not appear in JSON output
	_, hasPassword := result[0]["password"]
	assert.False(t, hasPassword, "password must not appear in JSON output")
	// Insecure must not appear in JSON output
	_, hasInsecure := result[0]["insecure"]
	assert.False(t, hasInsecure, "insecure must not appear in JSON output")
}

func TestDbmsCredentials_SetEmbed(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		dbmsName    string
		embedName   string
		wantErr     string
		wantValue   string
	}{
		{
			name:        "set embed link on existing credential",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			dbmsName:    "local",
			embedName:   "openai-default",
			wantErr:     "",
			wantValue:   "openai-default",
		},
		{
			name:        "clear embed link with empty embedName",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","embed-credential":"openai-default"}]}}`,
			dbmsName:    "local",
			embedName:   "",
			wantErr:     "",
			wantValue:   "",
		},
		{
			name:        "missing dbms credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`,
			dbmsName:    "nonexistent",
			embedName:   "openai-default",
			wantErr:     "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDbmsCredentials(t, tc.initialJSON)
			err := creds.Dbms.SetEmbed(tc.dbmsName, tc.embedName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			cred, getErr := creds.Dbms.Get(tc.dbmsName)
			require.NoError(t, getErr)
			assert.Equal(t, tc.wantValue, cred.EmbedCredential)
		})
	}
}

func TestDbmsCredentials_SetEmbed_Persists(t *testing.T) {
	// Verify that SetEmbed mutations persist via onUpdate callback
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]},"dbms":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"}]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Dbms.SetEmbed("local", "openai-default"))

	// Reload credentials from the same FS to verify persistence
	creds2 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.Len(t, creds2.Dbms.Credentials, 1)
	assert.Equal(t, "openai-default", creds2.Dbms.Credentials[0].EmbedCredential)

	// Now clear and verify the field is omitted on disk via omitempty
	require.NoError(t, creds2.Dbms.SetEmbed("local", ""))
	credsFile, err := afero.ReadFile(fs, filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(credsFile), "embed-credential", "embed-credential must be omitted on disk when empty (omitempty)")
}

func TestDbmsCredential_JSONRoundTrip_EmbedCredential(t *testing.T) {
	// On-disk: empty value omitted via omitempty; non-empty preserved.
	emptyCred := credentials.DbmsCredential{
		Name:         "local",
		Username:     "u",
		Password:     "p",
		DatabaseName: "neo4j",
		URI:          "bolt://localhost:7687",
	}
	data, err := json.Marshal(emptyCred)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "embed-credential", "empty EmbedCredential must be omitted on disk")

	linkedCred := credentials.DbmsCredential{
		Name:            "local",
		Username:        "u",
		Password:        "p",
		DatabaseName:    "neo4j",
		URI:             "bolt://localhost:7687",
		EmbedCredential: "openai-default",
	}
	data, err = json.Marshal(linkedCred)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"embed-credential":"openai-default"`)

	// Round-trip preserves the field
	var got credentials.DbmsCredential
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "openai-default", got.EmbedCredential)
}

func TestPrintableDbmsCredentials_AlwaysIncludesEmbedCredential(t *testing.T) {
	// Even when no credential has an embed link set, AsArray and JSON output
	// must always emit the `embed-credential` key (empty string) so the
	// column is stable for table rendering.
	creds, _ := newTestDbmsCredentials(t, `{"aura":{"credentials":[]},"dbms":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687"},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688","embed-credential":"openai-default"}]}}`)

	printable := creds.Dbms.Printable()
	rows := printable.AsArray()
	require.Len(t, rows, 2)

	v1, has1 := rows[0]["embed-credential"]
	assert.True(t, has1, "embed-credential key must always be present in AsArray")
	assert.Equal(t, "", v1, "embed-credential must be empty string when unset")

	v2, has2 := rows[1]["embed-credential"]
	assert.True(t, has2)
	assert.Equal(t, "openai-default", v2)

	// MarshalJSON also emits the key for both rows.
	data, err := json.Marshal(printable)
	require.NoError(t, err)
	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Len(t, parsed, 2)
	_, jsonHas1 := parsed[0]["embed-credential"]
	assert.True(t, jsonHas1, "embed-credential key must always be present in MarshalJSON")
	assert.Equal(t, "", parsed[0]["embed-credential"])
	assert.Equal(t, "openai-default", parsed[1]["embed-credential"])
}
