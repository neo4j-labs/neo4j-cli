// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/tidwall/gjson"
)

func TestEmbedCredentialRemove(t *testing.T) {
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
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
				{"name": "second", "provider": "ollama", "model": "m2", "base-url": "u2", "dimensions": 0, "api-key": ""},
			},
			initialDefault:  "first",
			command:         "remove second --yes --force",
			wantCredentials: `[{"name":"first","provider":"openai","model":"m","base-url":"u","dimensions":0,"api-key":"k1"}]`,
			wantDefault:     "first",
		},
		{
			name: "removing the default credential clears default-credential",
			initialCreds: []map[string]interface{}{
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
				{"name": "second", "provider": "ollama", "model": "m2", "base-url": "u2", "dimensions": 0, "api-key": ""},
			},
			initialDefault:  "first",
			command:         "remove first --yes --force",
			wantCredentials: `[{"name":"second","provider":"ollama","model":"m2","base-url":"u2","dimensions":0,"api-key":""}]`,
			wantDefault:     "",
		},
		{
			name: "unknown name returns descriptive error",
			initialCreds: []map[string]interface{}{
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
			},
			initialDefault: "first",
			command:        "remove nonexistent --yes --force",
			wantErr:        "could not find credential with name nonexistent to remove",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

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
			h.assertCredentialsValue("embed.default-credential", tc.wantDefault)
		})
	}
}

func TestEmbedCredentialRemove_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "credential embed remove",
		NoFlagsArgs:   "remove first",
		BothFlagsArgs: "remove first --yes --force",
		ResourceLabel: "embed",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			h := newEmbedTestHelper(t)
			h.setCredentialsValue("embed.credentials", []map[string]interface{}{
				{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
			})
			h.setStdin(stdin)
			err := h.executeCommand(args)
			file, ferr := h.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json"))
			if ferr != nil {
				t.Fatalf("open credentials: %v", ferr)
			}
			defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer
			contents, rerr := io.ReadAll(file)
			if rerr != nil {
				t.Fatalf("read credentials: %v", rerr)
			}
			invoked := gjson.Get(string(contents), "embed.credentials").String() == "[]"
			return confirmtest.GateRunResult{Err: err, Stderr: h.err.String(), Invoked: invoked}
		},
	})
}
