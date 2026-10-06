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
			name:        "header space value redacted whole",
			args:        []string{"aura", "api", "v1/instances", "--header", "Authorization: Bearer " + secret},
			flagNames:   []string{"--header"},
			mustContain: []string{"--header ***"},
		},
		{
			name:        "header equals value redacted whole",
			args:        []string{"aura", "api", "v1/instances", "--header=X-Api-Key: " + secret},
			flagNames:   []string{"--header"},
			mustContain: []string{"--header=***"},
		},
		{
			name:        "header shorthand space value redacted",
			args:        []string{"aura", "api", "v1/instances", "-H", "Authorization: Bearer " + secret},
			flagNames:   []string{"-H"},
			mustContain: []string{"-H ***"},
		},
		{
			name:        "header shorthand equals value redacted",
			args:        []string{"aura", "api", "v1/instances", "-H=Authorization: Bearer " + secret},
			flagNames:   []string{"-H"},
			mustContain: []string{"-H=***"},
		},
		{
			name:        "secret-named field redacted",
			args:        []string{"aura", "api", "v1/instances", "--method", "POST", "--field", "password=" + secret},
			flagNames:   []string{"--field"},
			mustContain: []string{"--field password=***", "--method POST"},
		},
		{
			name:        "secret-named field equals-on-flag form redacted",
			args:        []string{"aura", "api", "v1/x", "--field=client_secret=" + secret},
			flagNames:   []string{"--field"},
			mustContain: []string{"--field=client_secret=***"},
		},
		{
			name:        "non-secret field unchanged",
			args:        []string{"aura", "api", "v1/instances", "--field", "name=my-db", "--format", "json"},
			flagNames:   []string{"--field"},
			mustContain: []string{"--field name=my-db", "--format json"},
		},
		{
			name:        "secret-named raw-field redacted",
			args:        []string{"aura", "api", "v1/x", "--raw-field", "api_key=" + secret},
			flagNames:   []string{"--raw-field"},
			mustContain: []string{"--raw-field api_key=***"},
		},
		{
			name:        "non-secret raw-field unchanged",
			args:        []string{"aura", "api", "v1/x", "--raw-field=memory=2GB"},
			flagNames:   []string{"--raw-field"},
			mustContain: []string{"--raw-field=memory=2GB"},
		},
		{
			name:        "field shorthands redacted",
			args:        []string{"aura", "api", "v1/x", "-F", "token=" + secret, "-f", "secret=" + secret},
			flagNames:   []string{"-F", "-f"},
			mustContain: []string{"-F token=***", "-f secret=***"},
		},
		{
			name:        "force shorthand without kv value unchanged",
			args:        []string{"update", "-f", "--check"},
			mustContain: []string{"update -f --check"},
		},
		{
			name:        "header shorthand with attached value redacted",
			args:        []string{"aura", "api", "v1/x", "-HAuthorization: Bearer " + secret},
			flagNames:   []string{"-H"},
			mustContain: []string{"-H***"},
		},
		{
			name:        "field shorthand with attached secret value redacted",
			args:        []string{"aura", "api", "v1/x", "-Fpassword=" + secret},
			flagNames:   []string{"-F"},
			mustContain: []string{"-Fpassword=***"},
		},
		{
			name:        "raw-field shorthand with attached secret value redacted",
			args:        []string{"aura", "api", "v1/x", "-fclient_secret=" + secret},
			flagNames:   []string{"-f"},
			mustContain: []string{"-fclient_secret=***"},
		},
		{
			name:        "field shorthand with attached non-secret value unchanged",
			args:        []string{"aura", "api", "v1/x", "-Fname=my-db"},
			mustContain: []string{"-Fname=my-db"},
		},
		{
			name:        "password shorthand with attached value redacted",
			args:        []string{"query", "-p" + secret, "RETURN 1"},
			flagNames:   []string{"-p"},
			mustContain: []string{"-p***", "RETURN 1"},
		},
		{
			name:        "single-dash long form beats attached shorthand split",
			args:        []string{"credential", "add", "-password", secret},
			flagNames:   []string{"-password"},
			mustContain: []string{"-password ***"},
		},
		{
			name:        "single-dash long api-key form still consumes its value",
			args:        []string{"embed", "add", "-api-key", secret},
			flagNames:   []string{"-api-key"},
			mustContain: []string{"-api-key ***"},
		},
		{
			name:        "authorization field key redacted",
			args:        []string{"aura", "api", "v1/x", "--field", "authorization=" + secret},
			flagNames:   []string{"--field"},
			mustContain: []string{"--field authorization=***"},
		},
		{
			name:        "passphrase field key redacted",
			args:        []string{"aura", "api", "v1/x", "--raw-field", "passphrase=" + secret},
			flagNames:   []string{"--raw-field"},
			mustContain: []string{"--raw-field passphrase=***"},
		},
		{
			name:        "credential field key redacted",
			args:        []string{"aura", "api", "v1/x", "--field=credentials=" + secret},
			flagNames:   []string{"--field"},
			mustContain: []string{"--field=credentials=***"},
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
			name:    "basic header",
			in:      "Authorization: Basic dXNlcjpwYXNz",
			want:    "Authorization: Basic ***",
			mustNot: []string{"dXNlcjpwYXNz"},
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

// resetKnownSecrets clears the process-global registry so a test starts clean
// and restores it afterward (the registry is package state shared across tests).
func resetKnownSecrets(t *testing.T) {
	t.Helper()
	knownSecretsMu.Lock()
	saved := knownSecrets
	knownSecrets = nil
	knownSecretsMu.Unlock()
	t.Cleanup(func() {
		knownSecretsMu.Lock()
		knownSecrets = saved
		knownSecretsMu.Unlock()
	})
}

func TestRedactText_RegisteredSecretValue(t *testing.T) {
	const pw = "Abc123-generated-DB-password"

	t.Run("table cell value scrubbed", func(t *testing.T) {
		resetKnownSecrets(t)
		RegisterSecretValue(pw)
		// A horizontal box-drawing table: the value sits in its own column, on a
		// different line from the "PASSWORD" header — the shape-based regexes
		// cannot reach it, only the literal-match pass can.
		in := "│ PASSWORD                     │ USERNAME │\n│ " + pw + " │ neo4j    │"
		got := RedactText(in)
		if strings.Contains(got, pw) {
			t.Fatalf("registered secret leaked: %q", got)
		}
		if !strings.Contains(got, "***") {
			t.Fatalf("expected *** placeholder, got %q", got)
		}
		if !strings.Contains(got, "neo4j") {
			t.Errorf("non-secret cell over-redacted: %q", got)
		}
	})

	t.Run("unregistered output untouched", func(t *testing.T) {
		resetKnownSecrets(t)
		in := "│ NAME       │ USERNAME │\n│ Instance01 │ neo4j    │"
		if got := RedactText(in); got != in {
			t.Errorf("non-secret table over-redacted: %q", got)
		}
	})

	t.Run("short values ignored", func(t *testing.T) {
		resetKnownSecrets(t)
		RegisterSecretValue("abc") // < 4 chars
		in := "value abc here"
		if got := RedactText(in); got != in {
			t.Errorf("short value should not be registered/scrubbed: %q", got)
		}
	})

	t.Run("duplicate registration idempotent", func(t *testing.T) {
		resetKnownSecrets(t)
		RegisterSecretValue(pw)
		RegisterSecretValue(pw)
		knownSecretsMu.RLock()
		n := len(knownSecrets)
		knownSecretsMu.RUnlock()
		if n != 1 {
			t.Errorf("expected 1 registered secret, got %d", n)
		}
	})
}
