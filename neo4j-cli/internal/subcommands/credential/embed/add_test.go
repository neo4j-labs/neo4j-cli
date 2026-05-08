// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed_test

import (
	"testing"
)

func TestEmbedCredentialAdd(t *testing.T) {
	tests := []struct {
		name            string
		initialCreds    []map[string]interface{}
		initialDefault  string
		command         string
		wantErr         string
		wantCredentials string
		wantDefaultCred string
	}{
		{
			name:            "first credential is stored and set as default",
			initialCreds:    []map[string]interface{}{},
			initialDefault:  "",
			command:         "add --name openai-default --provider openai --model text-embedding-3-small --api-key sk-secret --base-url https://api.openai.com/v1 --dimensions 1536",
			wantCredentials: `[{"name":"openai-default","provider":"openai","model":"text-embedding-3-small","base-url":"https://api.openai.com/v1","dimensions":1536,"api-key":"sk-secret"}]`,
			wantDefaultCred: "openai-default",
		},
		{
			name:            "credential without optional flags is stored",
			initialCreds:    []map[string]interface{}{},
			initialDefault:  "",
			command:         "add --name local --provider ollama --model nomic-embed-text",
			wantCredentials: `[{"name":"local","provider":"ollama","model":"nomic-embed-text","base-url":"","dimensions":0,"api-key":""}]`,
			wantDefaultCred: "local",
		},
		{
			name: "duplicate name returns an error",
			initialCreds: []map[string]interface{}{
				{"name": "x", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k"},
			},
			initialDefault: "x",
			command:        "add --name x --provider openai --model m",
			wantErr:        "already have credential with name x",
		},
		{
			name: "second credential does not override existing default",
			initialCreds: []map[string]interface{}{
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k"},
			},
			initialDefault:  "first",
			command:         "add --name second --provider ollama --model m2",
			wantCredentials: `[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k"},{"name":"second","provider":"ollama","model":"m2","base-url":"","dimensions":0,"api-key":""}]`,
			wantDefaultCred: "first",
		},
		{
			name:         "invalid provider returns usage error naming the bad value",
			initialCreds: []map[string]interface{}{},
			command:      "add --name x --provider bogus --model m",
			wantErr:      `invalid --provider "bogus"`,
		},
		{
			name:         "missing --name produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --provider openai --model m",
			wantErr:      `required flag(s) "name" not set`,
		},
		{
			name:         "missing --provider produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name x --model m",
			wantErr:      `required flag(s) "provider" not set`,
		},
		{
			name:         "missing --model produces usage error",
			initialCreds: []map[string]interface{}{},
			command:      "add --name x --provider openai",
			wantErr:      `required flag(s) "model" not set`,
		},
		{
			name:            "huggingface provider is accepted",
			initialCreds:    []map[string]interface{}{},
			command:         "add --name hf --provider huggingface --model intfloat/e5-mistral-7b-instruct --api-key hf_secret",
			wantCredentials: `[{"name":"hf","provider":"huggingface","model":"intfloat/e5-mistral-7b-instruct","base-url":"","dimensions":0,"api-key":"hf_secret"}]`,
			wantDefaultCred: "hf",
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
			h.assertCredentialsValue("embed.credentials", tc.wantCredentials)
			h.assertCredentialsValue("embed.default-credential", tc.wantDefaultCred)
		})
	}
}
