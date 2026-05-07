// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed_test

import (
	"testing"
)

func TestEmbedCredentialUse(t *testing.T) {
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
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
				{"name": "second", "provider": "ollama", "model": "m2", "base-url": "u2", "dimensions": 0, "api-key": ""},
			},
			initialDefault: "first",
			command:        "use second",
			wantDefault:    "second",
		},
		{
			name: "unknown name returns descriptive error",
			initialCreds: []map[string]interface{}{
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
			},
			initialDefault: "first",
			command:        "use nonexistent",
			wantErr:        "could not find credential with name nonexistent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newEmbedTestHelper(t)
			h.setCredentialsValue("embed.credentials", tc.initialCreds)
			if tc.initialDefault != "" {
				h.setCredentialsValue("embed.default-credential", tc.initialDefault)
			}

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertCredentialsValue("embed.default-credential", tc.wantDefault)
		})
	}
}
