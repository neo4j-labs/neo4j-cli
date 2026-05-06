// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"testing"
)

func TestDbmsCredentialUse(t *testing.T) {
	tests := []struct {
		name           string
		initialCreds   []map[string]interface{}
		initialDefault string
		command        string
		wantErr        string
		wantDefault    string
	}{
		{
			name: "sets default credential by name",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
				{"name": "otherdb", "username": "neo4j", "password": "secret2", "database-name": "neo4j", "uri": "bolt://localhost:7688", "insecure": false},
			},
			initialDefault: "mydb",
			command:        "use otherdb",
			wantDefault:    "otherdb",
		},
		{
			name: "unknown name returns descriptive error",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
			},
			initialDefault: "mydb",
			command:        "use nonexistent",
			wantErr:        "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDbmsTestHelper(t)
			h.setCredentialsValue("dbms.credentials", tc.initialCreds)
			if tc.initialDefault != "" {
				h.setCredentialsValue("dbms.default-credential", tc.initialDefault)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("dbms.default-credential", tc.wantDefault)
		})
	}
}
