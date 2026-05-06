// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDbmsCredentialList(t *testing.T) {
	tests := []struct {
		name           string
		initialCreds   []map[string]interface{}
		initialDefault string
		command        string
		wantOut        func(t *testing.T, out string)
		wantErr        string
	}{
		{
			name:           "empty list returns empty JSON array",
			initialCreds:   []map[string]interface{}{},
			initialDefault: "",
			command:        "list --format json",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "[]")
			},
		},
		{
			name: "single credential listed as JSON",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
			},
			initialDefault: "mydb",
			command:        "list --format json",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "mydb"`)
				assert.Contains(t, out, `"username": "neo4j"`)
				assert.Contains(t, out, `"database-name": "neo4j"`)
				assert.Contains(t, out, `"uri": "bolt://localhost:7687"`)
				assert.Contains(t, out, `"default": true`)
				assert.NotContains(t, out, "secret")
				assert.NotContains(t, out, "password")
			},
		},
		{
			name: "multiple credentials listed as JSON with correct default",
			initialCreds: []map[string]interface{}{
				{"name": "first", "username": "neo4j", "password": "secret1", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
				{"name": "second", "username": "admin", "password": "secret2", "database-name": "mydb", "uri": "neo4j://remotehost:7687", "insecure": true},
			},
			initialDefault: "second",
			command:        "list --format json",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "first"`)
				assert.Contains(t, out, `"name": "second"`)
				assert.Contains(t, out, `"username": "neo4j"`)
				assert.Contains(t, out, `"username": "admin"`)
				assert.Contains(t, out, `"database-name": "mydb"`)
				assert.Contains(t, out, `"uri": "neo4j://remotehost:7687"`)
				// second is default, first is not
				assert.NotContains(t, out, "secret1")
				assert.NotContains(t, out, "secret2")
				assert.NotContains(t, out, "password")
			},
		},
		{
			name: "list as table shows correct columns",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
			},
			initialDefault: "mydb",
			command:        "list --format table",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "NAME")
				assert.Contains(t, out, "USERNAME")
				assert.Contains(t, out, "DATABASE-NAME")
				assert.Contains(t, out, "URI")
				assert.Contains(t, out, "INSECURE")
				assert.Contains(t, out, "DEFAULT")
				assert.Contains(t, out, "mydb")
				assert.Contains(t, out, "neo4j")
				assert.Contains(t, out, "bolt://localhost:7687")
				assert.NotContains(t, out, "secret")
				assert.NotContains(t, out, "password")
			},
		},
		{
			name:    "passing positional argument returns error",
			command: "list extra-arg",
			wantErr: `unknown command "extra-arg" for`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDbmsTestHelper(t)

			if tc.initialCreds != nil {
				h.setCredentialsValue("dbms.credentials", tc.initialCreds)
			}
			if tc.initialDefault != "" {
				h.setCredentialsValue("dbms.default-credential", tc.initialDefault)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			tc.wantOut(t, h.out.String())
		})
	}
}
