// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
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

// TestHandleResponseError_ExitCodeMapping locks the HTTP-status to typed-error
// mapping defined in REQ-F-004. Each subtest feeds a synthetic *http.Response
// to handleResponseError and asserts the returned error extracts a *CLIError
// with the expected Code via errors.As (working through any %w wrapping).
func TestHandleResponseError_ExitCodeMapping(t *testing.T) {
	// Build a minimal real cfg + credential for the 401/403/formatAuthorizationError
	// paths that touch cfg.Credentials.Aura.ClearAccessToken.
	newAuthFixture := func(t *testing.T) (*clicfg.Config, *credentials.AuraCredential) {
		t.Helper()
		fs, err := testfs.GetTestFs("{}", `{"aura":{"default-credential":"x","credentials":[{"name":"x","client-id":"id","client-secret":"secret"}]}}`)
		require.NoError(t, err)
		cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
		cred, err := cfg.Credentials.Aura.GetDefault()
		require.NoError(t, err)
		return cfg, cred
	}

	headerWithRetry := func(v string) http.Header {
		h := http.Header{}
		h.Set("Retry-After", v)
		return h
	}

	for _, tc := range []struct {
		name           string
		statusCode     int
		body           string
		header         http.Header
		wantCode       int
		wantMsgContain string // optional substring check on the rendered message
		usesAuthCfg    bool   // 401/403-no-server-error paths call ClearAccessToken
	}{
		{
			name:       "400 bad request -> validation (6)",
			statusCode: http.StatusBadRequest,
			body:       `{"errors":[{"message":"bad input","field":"name"}]}`,
			wantCode:   6,
		},
		{
			name:        "401 unauthorized -> auth (4)",
			statusCode:  http.StatusUnauthorized,
			body:        `{"errors":[{"message":"token invalid"}]}`,
			wantCode:    4,
			usesAuthCfg: true,
		},
		{
			name:       "403 forbidden with serverError -> auth (4)",
			statusCode: http.StatusForbidden,
			body:       `{"error":"forbidden endpoint"}`,
			wantCode:   4,
		},
		{
			name:        "403 forbidden without serverError -> auth (4)",
			statusCode:  http.StatusForbidden,
			body:        `{"errors":[{"message":"forbidden"}]}`,
			wantCode:    4,
			usesAuthCfg: true,
		},
		{
			name:       "404 not found -> not_found (3)",
			statusCode: http.StatusNotFound,
			body:       `{"errors":[{"message":"instance not found"}]}`,
			wantCode:   3,
		},
		{
			name:       "409 conflict -> conflict (5)",
			statusCode: http.StatusConflict,
			body:       `{"errors":[{"message":"already exists"}]}`,
			wantCode:   5,
		},
		{
			name:           "429 too many requests -> rate_limited (7) with Retry-After",
			statusCode:     http.StatusTooManyRequests,
			body:           `{}`,
			header:         headerWithRetry("30"),
			wantCode:       7,
			wantMsgContain: "30",
		},
		{
			name:       "500 internal server error -> upstream (8)",
			statusCode: http.StatusInternalServerError,
			body:       `{"errors":[{"message":"server boom"}]}`,
			wantCode:   8,
		},
		{
			name:       "503 service unavailable -> upstream (8)",
			statusCode: http.StatusServiceUnavailable,
			body:       `{"errors":[{"message":"unavailable"}]}`,
			wantCode:   8,
		},
		{
			name:       "405 method not allowed -> upstream (8)",
			statusCode: http.StatusMethodNotAllowed,
			body:       `{"errors":[{"message":"not allowed"}]}`,
			wantCode:   8,
		},
		{
			name:           "404 with malformed body -> fatal (1) with report-issue message",
			statusCode:     http.StatusNotFound,
			body:           `<<<not-json>>>`,
			wantCode:       1,
			wantMsgContain: "unexpected error [status 404]",
		},
		{
			name:           "502 with malformed body -> fatal (1) with report-issue message",
			statusCode:     http.StatusBadGateway,
			body:           `<<<not-json>>>`,
			wantCode:       1,
			wantMsgContain: "unexpected error [status 502]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := &http.Response{
				StatusCode: tc.statusCode,
				Body:       io.NopCloser(strings.NewReader(tc.body)),
				Header:     tc.header,
			}
			if res.Header == nil {
				res.Header = http.Header{}
			}

			var cfg *clicfg.Config
			var cred *credentials.AuraCredential
			if tc.usesAuthCfg {
				cfg, cred = newAuthFixture(t)
			}

			err := handleResponseError(res, cred, cfg)
			require.Error(t, err, "expected an error from handleResponseError")

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "errors.As should extract *CLIError; got %T: %v", err, err)
			assert.Equal(t, tc.wantCode, ce.Code, "exit code mismatch for status %d", tc.statusCode)

			if tc.wantMsgContain != "" {
				assert.Contains(t, ce.Error(), tc.wantMsgContain)
			}

			// 429 also asserts the Retry-After landed on the struct field.
			if tc.statusCode == http.StatusTooManyRequests {
				assert.Equal(t, tc.header.Get("Retry-After"), ce.RetryAfter, "RetryAfter struct field should mirror header")
			}
		})
	}
}
