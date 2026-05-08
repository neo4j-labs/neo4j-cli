// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRecoverPanic_RedactsSecretArgs verifies that the panic-recover path in
// main() never leaks secret-flag values into stdout. The recoverPanic helper
// re-panics so callers complete normal panic flow; the test recovers from that
// inner re-panic and asserts the previously-captured stdout output.
func TestRecoverPanic_RedactsSecretArgs(t *testing.T) {
	const secret = "S3CR3T-DO-NOT-LOG"

	for _, tc := range []struct {
		name string
		args []string
		want string // flag name that must appear redacted
	}{
		{
			name: "client-secret is masked",
			args: []string{"aura", "credential", "add", "--client-secret", secret},
			want: "--client-secret",
		},
		{
			name: "password equals form is masked",
			args: []string{"query", "--password=" + secret},
			want: "--password=",
		},
		{
			name: "api-key is masked",
			args: []string{"embed", "add", "--api-key", secret},
			want: "--api-key",
		},
		{
			name: "instance-password is masked",
			args: []string{"aura", "dataapi", "graphql", "create", "--instance-password", secret},
			want: "--instance-password",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			func() {
				defer func() {
					// recoverPanic re-panics; swallow the re-raise so the test
					// can inspect the captured stdout.
					_ = recover()
				}()
				recoverPanic(&out, tc.args, "boom")
			}()

			got := out.String()
			assert.Contains(t, got, tc.want, "redacted output should keep the flag NAME")
			assert.Contains(t, got, "***", "redacted output should contain *** placeholder")
			assert.NotContains(t, got, secret, "redacted output must NOT contain the secret value")
			assert.True(t, strings.Contains(got, "Unexpected error running CLI with args"), "expected the standard prefix line")
		})
	}
}
