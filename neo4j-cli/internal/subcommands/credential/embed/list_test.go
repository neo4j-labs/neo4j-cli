// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmbedCredentialList(t *testing.T) {
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
			name: "single credential listed as JSON omits api-key",
			initialCreds: []map[string]interface{}{
				{"name": "openai-default", "provider": "openai", "model": "text-embedding-3-small", "base-url": "https://api.openai.com/v1", "dimensions": 1536, "api-key": "sk-secret"},
			},
			initialDefault: "openai-default",
			command:        "list --format json",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "openai-default"`)
				assert.Contains(t, out, `"provider": "openai"`)
				assert.Contains(t, out, `"model": "text-embedding-3-small"`)
				assert.Contains(t, out, `"base_url": "https://api.openai.com/v1"`)
				assert.Contains(t, out, `"dimensions": 1536`)
				assert.Contains(t, out, `"default": true`)
				assert.NotContains(t, out, "sk-secret")
				assert.NotContains(t, out, "api-key")
			},
		},
		{
			name: "multiple credentials listed with correct default flagging",
			initialCreds: []map[string]interface{}{
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
				{"name": "second", "provider": "ollama", "model": "m2", "base-url": "u2", "dimensions": 0, "api-key": ""},
			},
			initialDefault: "second",
			command:        "list --format json",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "first"`)
				assert.Contains(t, out, `"name": "second"`)
				assert.NotContains(t, out, "k1")
				assert.NotContains(t, out, "api-key")
			},
		},
		{
			name: "list as table shows the documented six columns and never api-key",
			initialCreds: []map[string]interface{}{
				{"name": "openai-default", "provider": "openai", "model": "text-embedding-3-small", "base-url": "https://api.openai.com/v1", "dimensions": 1536, "api-key": "sk-secret"},
			},
			initialDefault: "openai-default",
			command:        "list --format table",
			wantOut: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "NAME")
				assert.Contains(t, out, "PROVIDER")
				assert.Contains(t, out, "MODEL")
				assert.Contains(t, out, "BASE_URL")
				assert.Contains(t, out, "DIMENSIONS")
				assert.Contains(t, out, "DEFAULT")
				assert.NotContains(t, out, "API-KEY")
				assert.Contains(t, out, "openai-default")
				assert.Contains(t, out, "openai")
				assert.NotContains(t, out, "sk-secret")
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
			h := newEmbedTestHelper(t)

			if tc.initialCreds != nil {
				h.setCredentialsValue("embed.credentials", tc.initialCreds)
			}
			if tc.initialDefault != "" {
				h.setCredentialsValue("embed.default-credential", tc.initialDefault)
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
