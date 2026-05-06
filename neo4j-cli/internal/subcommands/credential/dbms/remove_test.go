// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"testing"
)

func TestDbmsCredentialRemove(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]interface{}
		initialDefault  string
		command         string
		wantErr         string
		wantCredentials string
		wantDefault     string
	}{
		{
			name: "removes a non-default credential",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
				{"name": "otherdb", "username": "neo4j", "password": "secret2", "database-name": "neo4j", "uri": "bolt://localhost:7688", "insecure": false},
			},
			initialDefault:  "mydb",
			command:         "remove otherdb",
			wantCredentials: `[{"name":"mydb","username":"neo4j","password":"secret","database-name":"neo4j","uri":"bolt://localhost:7687","insecure":false}]`,
			wantDefault:     "mydb",
		},
		{
			name: "removing the default credential clears default-credential",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
				{"name": "otherdb", "username": "neo4j", "password": "secret2", "database-name": "neo4j", "uri": "bolt://localhost:7688", "insecure": false},
			},
			initialDefault:  "mydb",
			command:         "remove mydb",
			wantCredentials: `[{"name":"otherdb","username":"neo4j","password":"secret2","database-name":"neo4j","uri":"bolt://localhost:7688","insecure":false}]`,
			wantDefault:     "",
		},
		{
			name: "unknown name returns descriptive error",
			initialCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "insecure": false},
			},
			initialDefault: "mydb",
			command:        "remove nonexistent",
			wantErr:        "could not find credential with name nonexistent to remove",
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
			h.assertCredentialsValue("dbms.credentials", tc.wantCredentials)
			h.assertCredentialsValue("dbms.default-credential", tc.wantDefault)
		})
	}
}
