// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
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

// TestExitCodeFor verifies the standalone-aura main's helper for converting a
// returned error from cmd.Execute into a process exit code. Mirrors the test
// in neo4j-cli/main_test.go.
func TestExitCodeFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "nil error returns 0", err: nil, want: 0},
		{name: "plain error returns 1", err: errors.New("boom"), want: 1},
		{name: "fatal CLIError returns 1", err: clierr.NewFatalError("boom"), want: 1},
		{name: "usage CLIError returns 2", err: clierr.NewUsageError("bad flag"), want: 2},
		{name: "not-found CLIError returns 3", err: clierr.NewNotFoundError("missing"), want: 3},
		{name: "auth CLIError returns 4", err: clierr.NewAuthError("no token"), want: 4},
		{name: "conflict CLIError returns 5", err: clierr.NewConflictError("nope"), want: 5},
		{name: "validation CLIError returns 6", err: clierr.NewValidationError("bad input"), want: 6},
		{name: "rate-limit CLIError returns 7", err: clierr.NewRateLimitError("30", "slow down"), want: 7},
		{name: "upstream CLIError returns 8", err: clierr.NewUpstreamError("5xx"), want: 8},
		{
			name: "wrapped CLIError surfaces typed code through errors.As",
			err:  fmt.Errorf("outer: %w", clierr.NewNotFoundError("inner")),
			want: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, exitCodeFor(tc.err))
		})
	}
}
