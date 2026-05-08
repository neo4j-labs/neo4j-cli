// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRecoverPanic_RedactsSecretArgs verifies that the standalone-aura main()
// panic-recover path runs args through clievents.RedactArgs before printing.
// recoverPanic re-panics to preserve normal panic flow; the test recovers from
// the inner re-raise so it can inspect stdout.
func TestRecoverPanic_RedactsSecretArgs(t *testing.T) {
	const secret = "S3CR3T-DO-NOT-LOG"

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "client-secret is masked",
			args: []string{"credential", "add", "--client-secret", secret},
			want: "--client-secret",
		},
		{
			name: "instance-password equals form is masked",
			args: []string{"dataapi", "graphql", "create", "--instance-password=" + secret},
			want: "--instance-password=",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer

			func() {
				defer func() {
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
