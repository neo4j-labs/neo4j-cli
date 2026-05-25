// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
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

func TestEmbedCredentialRemoveConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h := newEmbedTestHelper(t)
	h.setCredentialsValue("embed.credentials", []map[string]interface{}{
		{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
	})

	err := h.executeCommand("remove first")

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 2 {
		t.Fatalf("err = %v, want *clierr.CLIError with exit 2", err)
	}
	h.assertCredentialsValue("embed.credentials", `[{"api-key":"k1","base-url":"u","dimensions":0,"model":"m","name":"first","provider":"openai"}]`)
}

func TestEmbedCredentialRemoveConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h := newEmbedTestHelper(t)
	h.setCredentialsValue("embed.credentials", []map[string]interface{}{
		{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
	})

	if err := h.executeCommand("remove first --yes --force"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.assertCredentialsValue("embed.credentials", `[]`)
}

func TestEmbedCredentialRemoveConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	h := newEmbedTestHelper(t)
	h.setCredentialsValue("embed.credentials", []map[string]interface{}{
		{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
	})
	h.setStdin("y\n")

	if err := h.executeCommand("remove first"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h.assertCredentialsValue("embed.credentials", `[]`)
	h.assertErr("Delete embed")
}

func TestEmbedCredentialRemoveConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	h := newEmbedTestHelper(t)
	h.setCredentialsValue("embed.credentials", []map[string]interface{}{
		{"name": "first", "provider": "openai", "model": "m", "base-url": "u", "dimensions": 0, "api-key": "k1"},
	})
	h.setStdin("N\n")

	if err := h.executeCommand("remove first"); err != nil {
		t.Fatalf("expected nil on cancel, got %v", err)
	}
	h.assertCredentialsValue("embed.credentials", `[{"api-key":"k1","base-url":"u","dimensions":0,"model":"m","name":"first","provider":"openai"}]`)
	h.assertErr("cancelled.")
}
