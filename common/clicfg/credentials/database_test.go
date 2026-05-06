// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDatabaseCredentials(t *testing.T, credentialsJSON string) (*credentials.Credentials, afero.Fs) {
	t.Helper()
	fs, err := testfs.GetTestFs("{}", credentialsJSON)
	require.NoError(t, err)
	cfg := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	return cfg, fs
}

func TestDatabaseCredentials_Add(t *testing.T) {
	tests := []struct {
		name          string
		initialJSON   string
		addName       string
		addUsername   string
		addPassword   string
		addDBName     string
		addURI        string
		addInsecure   bool
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
			addInsecure:   false,
			wantErr:       "",
			wantDefault:   "local",
			wantCredCount: 1,
		},
		{
			name:          "add second credential does not change default",
			initialJSON:   `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			addName:       "second",
			addUsername:   "u2",
			addPassword:   "p2",
			addDBName:     "test",
			addURI:        "bolt://localhost:7688",
			addInsecure:   true,
			wantErr:       "",
			wantDefault:   "first",
			wantCredCount: 2,
		},
		{
			name:          "duplicate name returns error",
			initialJSON:   `{"aura":{"credentials":[]},"database":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			addName:       "local",
			addUsername:   "u",
			addPassword:   "p",
			addDBName:     "neo4j",
			addURI:        "bolt://localhost:7687",
			addInsecure:   false,
			wantErr:       "already have credential with name local",
			wantDefault:   "local",
			wantCredCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDatabaseCredentials(t, tc.initialJSON)
			err := creds.Database.Add(tc.addName, tc.addUsername, tc.addPassword, tc.addDBName, tc.addURI, tc.addInsecure)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Database.DefaultCredential)
			assert.Len(t, creds.Database.Credentials, tc.wantCredCount)
		})
	}
}

func TestDatabaseCredentials_Remove(t *testing.T) {
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
			initialJSON:   `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688","insecure":false}]}}`,
			removeName:    "second",
			wantErr:       "",
			wantDefault:   "first",
			wantCredCount: 1,
		},
		{
			name:          "remove default credential clears default",
			initialJSON:   `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688","insecure":false}]}}`,
			removeName:    "first",
			wantErr:       "",
			wantDefault:   "",
			wantCredCount: 1,
		},
		{
			name:          "remove unknown credential returns error",
			initialJSON:   `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			removeName:    "nonexistent",
			wantErr:       "could not find credential with name nonexistent to remove",
			wantDefault:   "first",
			wantCredCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDatabaseCredentials(t, tc.initialJSON)
			err := creds.Database.Remove(tc.removeName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Database.DefaultCredential)
			assert.Len(t, creds.Database.Credentials, tc.wantCredCount)
		})
	}
}

func TestDatabaseCredentials_SetDefault(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		setName     string
		wantErr     string
		wantDefault string
	}{
		{
			name:        "set default to existing credential",
			initialJSON: `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688","insecure":false}]}}`,
			setName:     "second",
			wantErr:     "",
			wantDefault: "second",
		},
		{
			name:        "set default to unknown credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			setName:     "nonexistent",
			wantErr:     "could not find credential with name nonexistent",
			wantDefault: "first",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDatabaseCredentials(t, tc.initialJSON)
			err := creds.Database.SetDefault(tc.setName)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantDefault, creds.Database.DefaultCredential)
		})
	}
}

func TestDatabaseCredentials_GetDefault(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		wantNil     bool
		wantName    string
	}{
		{
			name:        "returns nil when no default set",
			initialJSON: `{"aura":{"credentials":[]},"database":{"credentials":[]}}`,
			wantNil:     true,
		},
		{
			name:        "returns default credential when set",
			initialJSON: `{"aura":{"credentials":[]},"database":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			wantNil:     false,
			wantName:    "local",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDatabaseCredentials(t, tc.initialJSON)
			cred, err := creds.Database.GetDefault()
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

func TestDatabaseCredentials_Get(t *testing.T) {
	tests := []struct {
		name        string
		initialJSON string
		getName     string
		wantErr     string
		wantName    string
	}{
		{
			name:        "get existing credential returns it",
			initialJSON: `{"aura":{"credentials":[]},"database":{"default-credential":"local","credentials":[{"name":"local","username":"user1","password":"pass1","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			getName:     "local",
			wantErr:     "",
			wantName:    "local",
		},
		{
			name:        "get unknown credential returns error",
			initialJSON: `{"aura":{"credentials":[]},"database":{"default-credential":"local","credentials":[{"name":"local","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`,
			getName:     "nonexistent",
			wantErr:     "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDatabaseCredentials(t, tc.initialJSON)
			cred, err := creds.Database.Get(tc.getName)
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

func TestDatabaseCredentials_List(t *testing.T) {
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
			initialJSON:   `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"u","password":"p","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false},{"name":"second","username":"u2","password":"p2","database-name":"test","uri":"bolt://localhost:7688","insecure":true}]}}`,
			wantCredCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds, _ := newTestDatabaseCredentials(t, tc.initialJSON)
			list := creds.Database.List()
			assert.Len(t, list, tc.wantCredCount)
		})
	}
}

func TestDatabaseCredentials_Persist(t *testing.T) {
	// Verify that mutations persist to the file via onUpdate callback
	fs, err := testfs.GetTestFs("{}", `{"aura":{"credentials":[]}}`)
	require.NoError(t, err)

	creds := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.NoError(t, creds.Database.Add("local", "neo4j", "secret", "neo4j", "bolt://localhost:7687", false))

	// Reload credentials from the same FS to verify persistence
	creds2 := credentials.NewCredentials(fs, clicfg.ConfigPrefix)
	require.Len(t, creds2.Database.Credentials, 1)
	assert.Equal(t, "local", creds2.Database.Credentials[0].Name)
	assert.Equal(t, "local", creds2.Database.DefaultCredential)
}

func TestPrintableDatabaseCredentials_AsArray(t *testing.T) {
	creds, _ := newTestDatabaseCredentials(t, `{"aura":{"credentials":[]},"database":{"default-credential":"first","credentials":[{"name":"first","username":"user1","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false},{"name":"second","username":"user2","password":"hidden","database-name":"test","uri":"bolt://localhost:7688","insecure":true}]}}`)

	printable := creds.Database.Printable()
	rows := printable.AsArray()

	require.Len(t, rows, 2)

	// First credential is the default
	assert.Equal(t, "first", rows[0]["name"])
	assert.Equal(t, "user1", rows[0]["username"])
	assert.Equal(t, "neo4j", rows[0]["database-name"])
	assert.Equal(t, "bolt://localhost:7687", rows[0]["uri"])
	assert.Equal(t, false, rows[0]["insecure"])
	assert.Equal(t, true, rows[0]["default"])
	// Password must not appear in output
	_, hasPassword := rows[0]["password"]
	assert.False(t, hasPassword, "password must not appear in AsArray output")

	// Second credential is not the default
	assert.Equal(t, "second", rows[1]["name"])
	assert.Equal(t, true, rows[1]["insecure"])
	assert.Equal(t, false, rows[1]["default"])
}

func TestPrintableDatabaseCredentials_MarshalJSON(t *testing.T) {
	creds, _ := newTestDatabaseCredentials(t, `{"aura":{"credentials":[]},"database":{"default-credential":"local","credentials":[{"name":"local","username":"user1","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]}}`)

	printable := creds.Database.Printable()
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
}
