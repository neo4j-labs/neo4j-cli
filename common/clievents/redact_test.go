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
			name:        "secret-named param space form redacted",
			args:        []string{"query", "MATCH (n) RETURN n", "--param", "token=" + secret},
			flagNames:   []string{"--param"},
			mustContain: []string{"--param token=***"},
		},
		{
			name:        "secret-named param equals-on-flag form redacted",
			args:        []string{"query", "--param=password=" + secret},
			flagNames:   []string{"--param"},
			mustContain: []string{"--param=password=***"},
		},
		{
			name:        "secret-named param embed modifier redacted",
			args:        []string{"query", "--param", "apiKey:embed=" + secret},
			flagNames:   []string{"--param"},
			mustContain: []string{"--param apiKey:embed=***"},
		},
		{
			name:        "non-secret param limit unchanged",
			args:        []string{"query", "--param", "limit=10", "--format", "json"},
			flagNames:   []string{"--param"},
			mustContain: []string{"--param limit=10", "--format json"},
		},
		{
			name:        "non-secret param name unchanged",
			args:        []string{"query", "--param", "name=bob"},
			flagNames:   []string{"--param"},
			mustContain: []string{"--param name=bob"},
		},
		{
			name:        "malformed param without equals unchanged",
			args:        []string{"query", "--param", "token"},
			flagNames:   []string{"--param"},
			mustContain: []string{"--param token"},
		},
		{
			name:        "uri followed by valueless secret flag does not leak",
			args:        []string{"query", "--uri", "--password", secret},
			flagNames:   []string{"--uri", "--password"},
			mustContain: []string{"--uri --password ***"},
		},
		{
			name:        "param followed by valueless secret flag does not leak",
			args:        []string{"query", "--param", "--password", secret},
			flagNames:   []string{"--param", "--password"},
			mustContain: []string{"--param --password ***"},
		},
		{
			name:        "generic secret flag redacts dash-leading value",
			args:        []string{"credential", "add", "--password", "-dashysecret"},
			flagNames:   []string{"--password"},
			mustContain: []string{"--password ***"},
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

func TestRedactText(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		mustNot []string
	}{
		{
			name: "uri userinfo password in free text",
			in:   "connecting to neo4j://neo4j:s3cret@host:7687 now",
			want: "connecting to neo4j://neo4j:***@host:7687 now",
		},
		{
			name: "https userinfo password",
			in:   "https://admin:hunter2@example.com/path",
			want: "https://admin:***@example.com/path",
		},
		{
			name: "uri without password unchanged",
			in:   "neo4j://host:7687/db",
			want: "neo4j://host:7687/db",
		},
		{
			name: "password assignment equals",
			in:   "password=topsecret",
			want: "password=***",
		},
		{
			name: "secret assignment colon",
			in:   "client_secret: abc123",
			want: "client_secret: ***",
		},
		{
			name: "token assignment",
			in:   "token=eyJhbGci",
			want: "token=***",
		},
		{
			name: "api-key assignment",
			in:   "api-key=sk-live-xyz",
			want: "api-key=***",
		},
		{
			name: "api_key underscore assignment",
			in:   "api_key=sk-live-xyz",
			want: "api_key=***",
		},
		{
			name: "auth assignment",
			in:   "auth=Zm9vOmJhcg==",
			want: "auth=***",
		},
		{
			name:    "json password field no space",
			in:      `{"password":"hunter2"}`,
			want:    `{"password":"***"}`,
			mustNot: []string{"hunter2"},
		},
		{
			name:    "json password field with space",
			in:      `{"password": "hunter2"}`,
			want:    `{"password": "***"}`,
			mustNot: []string{"hunter2"},
		},
		{
			name:    "json token field",
			in:      `{"token": "eyJhbGci.abc.def"}`,
			want:    `{"token": "***"}`,
			mustNot: []string{"eyJhbGci"},
		},
		{
			name:    "json client_secret field",
			in:      `{"client_secret":"abc123"}`,
			want:    `{"client_secret":"***"}`,
			mustNot: []string{"abc123"},
		},
		{
			name:    "json x-api-key field",
			in:      `{"x-api-key":"sk-live-xyz"}`,
			want:    `{"x-api-key":"***"}`,
			mustNot: []string{"sk-live-xyz"},
		},
		{
			name: "json non-secret name field unchanged",
			in:   `{"name":"bob"}`,
			want: `{"name":"bob"}`,
		},
		{
			name:    "json secret field among non-secret fields",
			in:      `{"id":"123","password":"p4ss","name":"bob"}`,
			want:    `{"id":"123","password":"***","name":"bob"}`,
			mustNot: []string{"p4ss"},
		},
		{
			name: "bearer header",
			in:   "Authorization: Bearer abc.def.ghi",
			want: "Authorization: Bearer ***",
		},
		{
			name: "non-secret assignment unchanged",
			in:   "limit=10",
			want: "limit=10",
		},
		{
			name: "ordinary prose unchanged",
			in:   "the quick brown fox ran 10 miles",
			want: "the quick brown fox ran 10 miles",
		},
		{
			name: "name assignment unchanged",
			in:   "name=bob",
			want: "name=bob",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name:    "multi-line with multiple secrets",
			in:      "uri=neo4j://u:p4ss@h:7687\npassword=hunter2\nAuthorization: Bearer tok-123\nlimit=5",
			want:    "uri=neo4j://u:***@h:7687\npassword=***\nAuthorization: Bearer ***\nlimit=5",
			mustNot: []string{"p4ss", "hunter2", "tok-123"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactText(tc.in)
			if got != tc.want {
				t.Errorf("RedactText(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for _, leak := range tc.mustNot {
				if strings.Contains(got, leak) {
					t.Errorf("secret %q leaked in output %q", leak, got)
				}
			}
		})
	}
}
