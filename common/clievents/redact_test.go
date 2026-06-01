// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"strings"
	"testing"
)

func TestRedactArgs(t *testing.T) {
	const secret = "s3cret-VALUE"

	tests := []struct {
		name        string
		args        []string
		flagNames   []string // flags that must appear in the output
		mustContain []string // substrings that must be present
	}{
		{
			name:        "long flag space value",
			args:        []string{"credential", "add", "--password", secret},
			flagNames:   []string{"--password"},
			mustContain: []string{"--password ***"},
		},
		{
			name:        "long flag equals value",
			args:        []string{"credential", "add", "--client-secret=" + secret},
			flagNames:   []string{"--client-secret"},
			mustContain: []string{"--client-secret=***"},
		},
		{
			name:        "single-dash defensive form",
			args:        []string{"embed", "add", "-api-key", secret},
			flagNames:   []string{"-api-key"},
			mustContain: []string{"-api-key ***"},
		},
		{
			name: "multiple secret flags",
			args: []string{
				"credential", "add",
				"--password", secret,
				"--client-secret=" + secret,
				"--api-key", secret,
				"--instance-password=" + secret,
			},
			flagNames: []string{"--password", "--client-secret", "--api-key", "--instance-password"},
			mustContain: []string{
				"--password ***",
				"--client-secret=***",
				"--api-key ***",
				"--instance-password=***",
			},
		},
		{
			name:        "secret as final arg with no following value",
			args:        []string{"credential", "add", "--password"},
			flagNames:   []string{"--password"},
			mustContain: []string{"--password"},
		},
		{
			name:        "no secret flag passes through",
			args:        []string{"aura", "instance", "list", "--format", "json"},
			mustContain: []string{"aura instance list --format json"},
		},
		{
			name:        "value-less flag does not consume next non-secret",
			args:        []string{"aura", "--help", "list"},
			mustContain: []string{"aura --help list"},
		},
		{
			name:        "mixed positional and secret flag",
			args:        []string{"dataapi", "graphql", "create", "my-api", "--instance-password", secret, "--format", "json"},
			flagNames:   []string{"--instance-password"},
			mustContain: []string{"my-api", "--instance-password ***", "--format json"},
		},
		{
			name:        "password shorthand space value",
			args:        []string{"query", "-p", secret, "MATCH (n) RETURN n"},
			flagNames:   []string{"-p"},
			mustContain: []string{"-p ***"},
		},
		{
			name:        "password shorthand equals value",
			args:        []string{"query", "-p=" + secret},
			flagNames:   []string{"-p"},
			mustContain: []string{"-p=***"},
		},
		{
			name:        "uri space value with userinfo password",
			args:        []string{"query", "--uri", "neo4j://u:" + secret + "@h:7687"},
			flagNames:   []string{"--uri"},
			mustContain: []string{"--uri neo4j://u:***@h:7687"},
		},
		{
			name:        "uri equals value with userinfo password",
			args:        []string{"query", "--uri=neo4j://u:" + secret + "@h"},
			flagNames:   []string{"--uri"},
			mustContain: []string{"--uri=neo4j://u:***@h"},
		},
		{
			name:        "uri without credentials unchanged",
			args:        []string{"query", "--uri", "neo4j://h:7687"},
			flagNames:   []string{"--uri"},
			mustContain: []string{"--uri neo4j://h:7687"},
		},
		{
			name:        "uri non-url value unchanged",
			args:        []string{"query", "--uri", "garbage"},
			flagNames:   []string{"--uri"},
			mustContain: []string{"--uri garbage"},
		},
		{
			name:        "empty args",
			args:        []string{},
			mustContain: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactArgs(tc.args)

			// Negative assertion: the secret value must never leak.
			if strings.Contains(got, secret) {
				t.Fatalf("secret value leaked in output: %q (got %q)", secret, got)
			}

			// Flag names must still be visible (so operators can see WHICH
			// flag was used, just not the value).
			for _, fn := range tc.flagNames {
				if !strings.Contains(got, fn) {
					t.Errorf("expected flag name %q in output, got %q", fn, got)
				}
			}

			for _, sub := range tc.mustContain {
				if !strings.Contains(got, sub) {
					t.Errorf("expected substring %q in output, got %q", sub, got)
				}
			}
		})
	}
}
