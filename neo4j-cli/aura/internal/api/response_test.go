// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleResponseError_RedactsSecretArgs verifies that every panic site in
// handleResponseError that interpolates os.Args[1:] runs the value through
// clievents.RedactArgs first, so secrets like --client-secret / --password /
// --api-key / --instance-password never leak into the panic message.
func TestHandleResponseError_RedactsSecretArgs(t *testing.T) {
	const secret = "S3CR3T-VALUE-DO-NOT-LOG"

	for _, tc := range []struct {
		name           string
		statusCode     int
		body           string
		args           []string
		wantContains   []string // substrings that MUST appear in the recovered panic
		wantFlagInArgs string   // flag name that must appear in the redacted args portion
	}{
		{
			name:           "415 unsupported media type with --client-secret",
			statusCode:     http.StatusUnsupportedMediaType,
			body:           ``,
			args:           []string{"credential", "add", "--name", "x", "--client-id", "id", "--client-secret", secret},
			wantContains:   []string{"unexpected error", "--client-secret", "***"},
			wantFlagInArgs: "--client-secret",
		},
		{
			name:           "502 bad gateway with malformed body and --password",
			statusCode:     http.StatusBadGateway,
			body:           `<<<not-json>>>`,
			args:           []string{"dbms", "add", "--password", secret},
			wantContains:   []string{"unexpected error", "--password", "***"},
			wantFlagInArgs: "--password",
		},
		{
			name:           "307 permanent redirect with --instance-password",
			statusCode:     http.StatusPermanentRedirect,
			body:           ``,
			args:           []string{"dataapi", "graphql", "create", "--instance-password", secret},
			wantContains:   []string{"unexpected error", "--instance-password", "***"},
			wantFlagInArgs: "--instance-password",
		},
		{
			name:           "400 bad request with malformed body and --api-key",
			statusCode:     http.StatusBadRequest,
			body:           `}}{not-json{{`,
			args:           []string{"embed", "add", "--api-key", secret},
			wantContains:   []string{"unexpected error", "--api-key", "***"},
			wantFlagInArgs: "--api-key",
		},
		{
			name:           "default branch (unknown 599) with --client-secret",
			statusCode:     599,
			body:           `irrelevant`,
			args:           []string{"credential", "add", "--client-secret", secret},
			wantContains:   []string{"unexpected status code", "--client-secret", "***"},
			wantFlagInArgs: "--client-secret",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origArgs := os.Args
			t.Cleanup(func() { os.Args = origArgs })
			// Mimic os.Args layout: index 0 is the binary name; index 1+ is what RedactArgs sees.
			os.Args = append([]string{"neo4j-cli"}, tc.args...)

			res := &http.Response{
				StatusCode: tc.statusCode,
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}

			defer func() {
				r := recover()
				require.NotNil(t, r, "expected handleResponseError to panic for status %d", tc.statusCode)

				var msg string
				switch v := r.(type) {
				case error:
					msg = v.Error()
				case string:
					msg = v
				default:
					t.Fatalf("unexpected panic value type: %T", r)
				}

				for _, want := range tc.wantContains {
					assert.Contains(t, msg, want, "panic message should contain %q", want)
				}
				assert.NotContains(t, msg, secret, "panic message must NOT contain the raw secret value")
			}()

			// nil credential / nil cfg are unused on the panic paths we exercise.
			_ = handleResponseError(res, nil, nil)
			t.Fatalf("expected panic for status %d, none occurred", tc.statusCode)
		})
	}
}
