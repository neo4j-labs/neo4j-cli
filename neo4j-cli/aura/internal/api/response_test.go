// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
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
		name               string
		statusCode         int
		body               string
		header             http.Header
		wantCode           int
		wantMsgContain     string // optional substring check on the rendered message
		wantSuggestion     string // optional Suggestion field check (errors.As)
		assertNoSuggestion bool   // when set, assert Suggestion == "" even though wantSuggestion is empty
		usesAuthCfg        bool   // 401/403-no-server-error paths call ClearAccessToken
	}{
		{
			name:           "400 bad request -> validation (6)",
			statusCode:     http.StatusBadRequest,
			body:           `{"errors":[{"message":"bad input","field":"name"}]}`,
			wantCode:       6,
			wantSuggestion: "See 'neo4j-cli aura <cmd> --help' for valid flags and values.",
		},
		{
			name:           "401 unauthorized -> auth (4)",
			statusCode:     http.StatusUnauthorized,
			body:           `{"errors":[{"message":"token invalid"}]}`,
			wantCode:       4,
			usesAuthCfg:    true,
			wantSuggestion: "Run 'neo4j-cli credential aura-client add' to refresh credentials, then retry.",
		},
		{
			name:           "403 forbidden with serverError -> auth (4)",
			statusCode:     http.StatusForbidden,
			body:           `{"error":"forbidden endpoint"}`,
			wantCode:       4,
			wantSuggestion: "Run 'neo4j-cli credential aura-client add' to refresh credentials, then retry.",
		},
		{
			name:           "403 forbidden without serverError -> auth (4)",
			statusCode:     http.StatusForbidden,
			body:           `{"errors":[{"message":"forbidden"}]}`,
			wantCode:       4,
			usesAuthCfg:    true,
			wantSuggestion: "Run 'neo4j-cli credential aura-client add' to refresh credentials, then retry.",
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
			name:           "402 payment required quota-exceeded -> conflict (5) with quota suggestion",
			statusCode:     http.StatusPaymentRequired,
			body:           `{"errors":[{"message":"User is not permitted to create any more instances of this type.","reason":"quota-exceeded"}]}`,
			wantCode:       5,
			wantMsgContain: "User is not permitted to create any more instances of this type.",
			wantSuggestion: "You've reached your quota for this instance type. Delete an existing instance with 'neo4j-cli aura instance list' then 'neo4j-cli aura instance delete <id>', or pick a different --type.",
		},
		{
			name:               "402 payment required other reason -> conflict (5) no suggestion",
			statusCode:         http.StatusPaymentRequired,
			body:               `{"errors":[{"message":"payment declined","reason":"something-else"}]}`,
			wantCode:           5,
			wantMsgContain:     "payment declined",
			assertNoSuggestion: true,
		},
		{
			name:           "402 with malformed body -> fatal (1) with report-issue message",
			statusCode:     http.StatusPaymentRequired,
			body:           `<<<not-json>>>`,
			wantCode:       1,
			wantMsgContain: "unexpected error [status 402]",
		},
		{
			name:           "429 too many requests -> rate_limited (7) with Retry-After",
			statusCode:     http.StatusTooManyRequests,
			body:           `{}`,
			header:         headerWithRetry("30"),
			wantCode:       7,
			wantMsgContain: "30",
			wantSuggestion: "Retry after 30 seconds.",
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

			if tc.wantSuggestion != "" {
				assert.Equal(t, tc.wantSuggestion, ce.Suggestion, "Suggestion mismatch for status %d", tc.statusCode)
			}

			if tc.assertNoSuggestion {
				assert.Equal(t, "", ce.Suggestion, "Suggestion should be empty for status %d", tc.statusCode)
			}

			// 429 also asserts the Retry-After landed on the struct field.
			if tc.statusCode == http.StatusTooManyRequests {
				assert.Equal(t, tc.header.Get("Retry-After"), ce.RetryAfter, "RetryAfter struct field should mirror header")
			}
		})
	}
}

// TestParseResourceFromRequest exercises the URL-path -> (resourceType,
// resourceID) helper used by the 404 branch. Aura paths follow
// `/<version>/<plural>/<id>[/...]`; unrecognised or short paths must yield
// empty strings so the envelope omitempty drops both fields.
func TestParseResourceFromRequest(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     string
		wantType string
		wantID   string
	}{
		{name: "v1 instances with id", path: "/v1/instances/abc123", wantType: "instance", wantID: "abc123"},
		{name: "v1 tenants with id", path: "/v1/tenants/tnt-1", wantType: "tenant", wantID: "tnt-1"},
		{name: "v1beta5 instances with nested subpath", path: "/v1beta5/instances/abc/data-apis/graphql", wantType: "instance", wantID: "abc"},
		{name: "v1 customer-managed-keys with id", path: "/v1/customer-managed-keys/cmk-1", wantType: "customer-managed-key", wantID: "cmk-1"},
		{name: "v1 snapshots nested via instance", path: "/v1/instances/i-1/snapshots/snap-1", wantType: "instance", wantID: "i-1"},
		{name: "no id segment", path: "/v1/instances", wantType: "", wantID: ""},
		{name: "version only", path: "/v1", wantType: "", wantID: ""},
		{name: "empty path", path: "", wantType: "", wantID: ""},
		{name: "root", path: "/", wantType: "", wantID: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{URL: &url.URL{Path: tc.path}}
			gotType, gotID := parseResourceFromRequest(req)
			assert.Equal(t, tc.wantType, gotType, "resource type mismatch")
			assert.Equal(t, tc.wantID, gotID, "resource id mismatch")
		})
	}

	t.Run("nil request returns empties", func(t *testing.T) {
		gotType, gotID := parseResourceFromRequest(nil)
		assert.Equal(t, "", gotType)
		assert.Equal(t, "", gotID)
	})

	t.Run("nil URL returns empties", func(t *testing.T) {
		gotType, gotID := parseResourceFromRequest(&http.Request{})
		assert.Equal(t, "", gotType)
		assert.Equal(t, "", gotID)
	})
}

// TestHandleResponseError_NotFound_TagsResource locks the 404 branch: a
// `/v1/instances/{id}` request that returns 404 must produce a *CLIError
// whose ResourceType="instance" and ResourceID matches the path segment so
// the JSON envelope surfaces them under resource_type / resource_id. It also
// locks the per-resource Suggestion attached via suggestionForResource(...).
func TestHandleResponseError_NotFound_TagsResource(t *testing.T) {
	for _, tc := range []struct {
		name           string
		path           string
		wantType       string
		wantID         string
		wantSuggestion string
	}{
		{
			name:           "instance 404 tagged with type+id+suggestion",
			path:           "/v1/instances/inst-404",
			wantType:       "instance",
			wantID:         "inst-404",
			wantSuggestion: "Run 'neo4j-cli aura instance list' to see available instances.",
		},
		{
			name:           "tenant 404 tagged with type+id+migration suggestion",
			path:           "/v1/tenants/tnt-404",
			wantType:       "tenant",
			wantID:         "tnt-404",
			wantSuggestion: "Run 'neo4j-cli aura project list' to see available projects (tenants are now called projects).",
		},
		{
			name:           "unrecognised short path leaves resource fields and suggestion empty",
			path:           "/v1/instances",
			wantType:       "",
			wantID:         "",
			wantSuggestion: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{URL: &url.URL{Path: tc.path}}
			res := &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"errors":[{"message":"not found"}]}`)),
				Header:     http.Header{},
				Request:    req,
			}

			err := handleResponseError(res, nil, nil)
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "errors.As should extract *CLIError; got %T: %v", err, err)
			assert.Equal(t, 3, ce.Code, "404 must map to exit 3 (not_found)")
			assert.Equal(t, tc.wantType, ce.ResourceType, "ResourceType mismatch")
			assert.Equal(t, tc.wantID, ce.ResourceID, "ResourceID mismatch")
			assert.Equal(t, tc.wantSuggestion, ce.Suggestion, "Suggestion mismatch")
		})
	}
}

// TestSuggestionForResource locks the per-resource 404 suggestion lookup. The
// lookup table is keyed on the singular resourceType produced by
// parseResourceFromRequest; unknown / empty types must return "" so the
// envelope omitempty drops the field.
func TestSuggestionForResource(t *testing.T) {
	for _, tc := range []struct {
		name         string
		resourceType string
		want         string
	}{
		{
			name:         "instance",
			resourceType: "instance",
			want:         "Run 'neo4j-cli aura instance list' to see available instances.",
		},
		{
			name:         "project",
			resourceType: "project",
			want:         "Run 'neo4j-cli aura project list --organization-id <id>' to see available projects.",
		},
		{
			name:         "organization",
			resourceType: "organization",
			want:         "Run 'neo4j-cli aura organization list' to see available organizations.",
		},
		{
			name:         "customer-managed-key",
			resourceType: "customer-managed-key",
			want:         "Run 'neo4j-cli aura customer-managed-key list' to see customer-managed keys.",
		},
		{
			name:         "tenant migration nudge",
			resourceType: "tenant",
			want:         "Run 'neo4j-cli aura project list' to see available projects (tenants are now called projects).",
		},
		{
			name:         "unknown resource type",
			resourceType: "graph-analytic",
			want:         "",
		},
		{
			name:         "empty resource type",
			resourceType: "",
			want:         "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, suggestionForResource(tc.resourceType))
		})
	}
}

// TestSuggestionForPaymentRequired locks the 402 quota-exceeded suggestion
// lookup. Any error entry with Reason == "quota-exceeded" triggers the quota
// suggestion; everything else (unknown reason / empty errors[]) returns "" so
// the envelope omitempty drops the field. Multi-error bodies still trigger as
// long as one entry matches.
func TestSuggestionForPaymentRequired(t *testing.T) {
	const quotaSuggestion = "You've reached your quota for this instance type. Delete an existing instance with 'neo4j-cli aura instance list' then 'neo4j-cli aura instance delete <id>', or pick a different --type."

	for _, tc := range []struct {
		name string
		resp ErrorResponse
		want string
	}{
		{
			name: "positive quota-exceeded",
			resp: ErrorResponse{Errors: []Error{{Message: "User is not permitted to create any more instances of this type.", Reason: "quota-exceeded"}}},
			want: quotaSuggestion,
		},
		{
			name: "negative unknown reason",
			resp: ErrorResponse{Errors: []Error{{Message: "payment declined", Reason: "card-declined"}}},
			want: "",
		},
		{
			name: "empty errors slice",
			resp: ErrorResponse{Errors: nil},
			want: "",
		},
		{
			name: "multi-error body with one quota-exceeded entry",
			resp: ErrorResponse{Errors: []Error{
				{Message: "other failure", Reason: "something-else"},
				{Message: "User is not permitted to create any more instances of this type.", Reason: "quota-exceeded"},
			}},
			want: quotaSuggestion,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, suggestionForPaymentRequired(tc.resp))
		})
	}
}
