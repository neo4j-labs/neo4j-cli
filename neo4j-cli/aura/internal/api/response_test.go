// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleResponseError_RedactsBodySecrets asserts a secret echoed back by
// the API in the response body does not survive into the error message, which
// is persisted by the tee-on-failure path. The secret now arrives in the body
// rather than os.Args; every status that reaches upstreamDetail must run the
// body through scrubbedBodyTrunc (RedactText + StripControl).
func TestHandleResponseError_RedactsBodySecrets(t *testing.T) {
	const secret = "S3CR3T-VALUE-DO-NOT-LOG"

	for _, tc := range []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "415 unsupported media type with secret in body",
			statusCode: http.StatusUnsupportedMediaType,
			body:       `{"password":"` + secret + `"}`,
		},
		{
			name:       "307 permanent redirect with secret in body",
			statusCode: http.StatusPermanentRedirect,
			body:       `{"password":"` + secret + `"}`,
		},
		{
			name:       "400 bad request with body containing secret",
			statusCode: http.StatusBadRequest,
			body:       `{"password":"` + secret + `"}`,
		},
		{
			name:       "599 unknown status with secret in body",
			statusCode: 599,
			body:       `{"password":"` + secret + `"}`,
		},
		{
			name:       "413 payload too large with secret in body",
			statusCode: http.StatusRequestEntityTooLarge,
			body:       `{"password":"` + secret + `"}`,
		},
		{
			name:       "422 unprocessable entity with secret in body",
			statusCode: http.StatusUnprocessableEntity,
			body:       `{"password":"` + secret + `"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := &http.Response{
				StatusCode: tc.statusCode,
				Body:       io.NopCloser(strings.NewReader(tc.body)),
				Header:     http.Header{},
			}

			err := handleResponseError(res, nil, nil)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), secret)
			assert.Contains(t, err.Error(), "***")
		})
	}
}

// errReader implements io.Reader, always returning an error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestHandleResponseError_ReadAllFailure verifies that a body read failure
// returns an upstream error (8) instead of panicking.
func TestHandleResponseError_ReadAllFailure(t *testing.T) {
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(errReader{}),
		Header:     http.Header{},
	}
	err := handleResponseError(res, nil, nil)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 8, ce.Code, "body read failure should map to exit 8 (upstream)")
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
		requestPath        string // when set, attaches a Request with this URL path
		wantCode           int
		wantMsgContain     string // optional substring check on the rendered message
		wantMsgOmit        string // optional substring that must NOT appear in the message
		wantSuggestion     string // optional Suggestion field check (errors.As)
		wantResourceType   string // optional ResourceType check
		wantResourceID     string // optional ResourceID check
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
			name:       "402 with malformed body -> conflict (5)",
			statusCode: http.StatusPaymentRequired,
			body:       `<<<not-json>>>`,
			wantCode:   5,
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
			name:       "404 with malformed body -> not_found (3)",
			statusCode: http.StatusNotFound,
			body:       `<<<not-json>>>`,
			wantCode:   3,
		},
		{
			name:       "502 with malformed body -> upstream (8)",
			statusCode: http.StatusBadGateway,
			body:       `<<<not-json>>>`,
			wantCode:   8,
		},
		{
			name:       "307 permanent redirect -> upstream (8)",
			statusCode: http.StatusPermanentRedirect,
			body:       `{}`,
			wantCode:   8,
		},
		{
			name:       "413 payload too large -> validation (6)",
			statusCode: http.StatusRequestEntityTooLarge,
			body:       `{}`,
			wantCode:   6,
		},
		{
			name:       "422 unprocessable entity -> validation (6)",
			statusCode: http.StatusUnprocessableEntity,
			body:       `{}`,
			wantCode:   6,
		},
		{
			name:       "408 request timeout (unmapped 4xx) -> validation (6)",
			statusCode: http.StatusRequestTimeout,
			body:       `{}`,
			wantCode:   6,
		},
		// ---------- Group A: additional status and malformed-body entries ----------
		{
			name:       "400 bad request with malformed body -> validation (6)",
			statusCode: http.StatusBadRequest,
			body:       `<<<not-json>>>`,
			wantCode:   6,
		},
		{
			name:           "403 forbidden with malformed body -> auth (4) with authSuggestion",
			statusCode:     http.StatusForbidden,
			body:           `<<<not-json>>>`,
			wantCode:       4,
			wantSuggestion: authSuggestion,
		},
		{
			name:       "413 payload too large with parseable errors[] -> validation (6)",
			statusCode: http.StatusRequestEntityTooLarge,
			body:       `{"errors":[{"message":"payload too large"}]}`,
			wantCode:   6,
		},
		{
			name:       "415 unsupported media type with schema-less body -> validation (6)",
			statusCode: http.StatusUnsupportedMediaType,
			body:       `{}`,
			wantCode:   6,
		},
		{
			name:           "422 unprocessable entity with parseable errors[] -> validation (6) surfacing message",
			statusCode:     http.StatusUnprocessableEntity,
			body:           `{"errors":[{"message":"database limit reached for this instance"}]}`,
			wantCode:       6,
			wantMsgContain: "database limit reached for this instance",
		},
		{
			name:       "422 unprocessable entity with schema-less plain-text body -> validation (6)",
			statusCode: http.StatusUnprocessableEntity,
			body:       `plain text error body`,
			wantCode:   6,
		},
		{
			name:       "405 method not allowed with malformed body -> upstream (8)",
			statusCode: http.StatusMethodNotAllowed,
			body:       `<<<not-json>>>`,
			wantCode:   8,
		},
		{
			name:       "409 conflict with malformed body -> conflict (5)",
			statusCode: http.StatusConflict,
			body:       `<<<not-json>>>`,
			wantCode:   5,
		},
		{
			name:       "599 unknown status -> upstream (8)",
			statusCode: 599,
			body:       `{}`,
			wantCode:   8,
		},
		{
			name:       "451 unavailable for legal reasons (unmapped 4xx) -> validation (6)",
			statusCode: 451,
			body:       `{}`,
			wantCode:   6,
		},
		// ---------- Group B: empty-errors bodies proving REQ-F-004 ----------
		// Both '{}' and '{"errors":[]}' parse as valid JSON leaving an empty
		// Errors slice, so errorMessages returns nil and the message falls back
		// to upstreamDetail rather than rendering the empty "[\n\t\n]".
		{
			name:           "400 empty json body -> validation (6) with raw body fallback",
			statusCode:     http.StatusBadRequest,
			body:           `{}`,
			wantCode:       6,
			wantMsgContain: "upstream error [status 400]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:           "400 empty errors[] body -> validation (6) with raw body fallback",
			statusCode:     http.StatusBadRequest,
			body:           `{"errors":[]}`,
			wantCode:       6,
			wantMsgContain: "upstream error [status 400]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:           "402 empty json body -> conflict (5) with raw body fallback",
			statusCode:     http.StatusPaymentRequired,
			body:           `{}`,
			wantCode:       5,
			wantMsgContain: "upstream error [status 402]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:           "402 empty errors[] body -> conflict (5) with raw body fallback",
			statusCode:     http.StatusPaymentRequired,
			body:           `{"errors":[]}`,
			wantCode:       5,
			wantMsgContain: "upstream error [status 402]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:             "404 empty json body -> not_found (3) with resource tags and raw body",
			statusCode:       http.StatusNotFound,
			body:             `{}`,
			requestPath:      "/v1/instances/inst-1",
			wantCode:         3,
			wantMsgContain:   "upstream error [status 404]",
			wantMsgOmit:      "[\n\t\n]",
			wantResourceType: "instance",
			wantResourceID:   "inst-1",
			wantSuggestion:   "Run 'neo4j-cli aura instance list' to see available instances.",
		},
		{
			name:             "404 empty errors[] body -> not_found (3) with resource tags and raw body",
			statusCode:       http.StatusNotFound,
			body:             `{"errors":[]}`,
			requestPath:      "/v1/instances/inst-1",
			wantCode:         3,
			wantMsgContain:   "upstream error [status 404]",
			wantMsgOmit:      "[\n\t\n]",
			wantResourceType: "instance",
			wantResourceID:   "inst-1",
			wantSuggestion:   "Run 'neo4j-cli aura instance list' to see available instances.",
		},
		{
			name:           "409 empty json body -> conflict (5) with raw body fallback",
			statusCode:     http.StatusConflict,
			body:           `{}`,
			wantCode:       5,
			wantMsgContain: "upstream error [status 409]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:           "409 empty errors[] body -> conflict (5) with raw body fallback",
			statusCode:     http.StatusConflict,
			body:           `{"errors":[]}`,
			wantCode:       5,
			wantMsgContain: "upstream error [status 409]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:           "500 empty json body -> upstream (8) with raw body fallback",
			statusCode:     http.StatusInternalServerError,
			body:           `{}`,
			wantCode:       8,
			wantMsgContain: "upstream error [status 500]",
			wantMsgOmit:    "[\n\t\n]",
		},
		{
			name:           "500 empty errors[] body -> upstream (8) with raw body fallback",
			statusCode:     http.StatusInternalServerError,
			body:           `{"errors":[]}`,
			wantCode:       8,
			wantMsgContain: "upstream error [status 500]",
			wantMsgOmit:    "[\n\t\n]",
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
			if tc.requestPath != "" {
				res.Request = &http.Request{URL: &url.URL{Path: tc.requestPath}}
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

			if tc.wantMsgOmit != "" {
				assert.NotContains(t, ce.Error(), tc.wantMsgOmit)
			}

			if tc.wantResourceType != "" {
				assert.Equal(t, tc.wantResourceType, ce.ResourceType, "ResourceType mismatch for status %d", tc.statusCode)
			}

			if tc.wantResourceID != "" {
				assert.Equal(t, tc.wantResourceID, ce.ResourceID, "ResourceID mismatch for status %d", tc.statusCode)
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
		{name: "v2beta1 nested instance with id", path: "/v2beta1/organizations/org-1/projects/proj-1/instances/inst-1", wantType: "instance", wantID: "inst-1"},
		{name: "v2beta1 nested session with id", path: "/v2beta1/organizations/org-1/projects/proj-1/graph-analytics/sessions/sess-1", wantType: "session", wantID: "sess-1"},
		{name: "v2beta1 nested instance list (no id)", path: "/v2beta1/organizations/org-1/projects/proj-1/instances", wantType: "", wantID: ""},
		{name: "v2beta1 nested instance action suffix", path: "/v2beta1/organizations/org-1/projects/proj-1/instances/inst-1/pause", wantType: "instance", wantID: "inst-1"},
		{name: "v2beta1 nested agent invoke action suffix", path: "/v2beta1/organizations/org-1/projects/proj-1/agents/agent-1/invoke", wantType: "agent", wantID: "agent-1"},
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

// TestHandleResponseError_NoPanic asserts the panic-freedom and closed-enum
// invariants across a broad status x body-shape matrix. It does NOT lock
// specific status-to-code mappings — that is
// TestHandleResponseError_ExitCodeMapping's job. Do not extend this test into
// a per-status mapping table; add cases there instead.
func TestHandleResponseError_NoPanic(t *testing.T) {
	authFs, err := testfs.GetTestFs("{}", `{"aura":{"default-credential":"x","credentials":[{"name":"x","client-id":"id","client-secret":"secret"}]}}`)
	require.NoError(t, err)
	authCfg := clicfg.NewConfig(authFs, "test", clicfg.AuraScope)
	authCred, err := authCfg.Credentials.Aura.GetDefault()
	require.NoError(t, err)

	statuses := []int{
		http.StatusPermanentRedirect,     // 308
		http.StatusBadRequest,            // 400
		http.StatusUnauthorized,          // 401
		http.StatusPaymentRequired,       // 402
		http.StatusForbidden,             // 403
		http.StatusNotFound,              // 404
		http.StatusMethodNotAllowed,      // 405
		http.StatusConflict,              // 409
		http.StatusRequestEntityTooLarge, // 413
		http.StatusUnsupportedMediaType,  // 415
		http.StatusUnprocessableEntity,   // 422
		http.StatusTooManyRequests,       // 429
		http.StatusInternalServerError,   // 500
		http.StatusBadGateway,            // 502
		http.StatusServiceUnavailable,    // 503
		http.StatusGatewayTimeout,        // 504
		// Unmapped samples from outside the handled set
		302, 408, 418, 451, 507, 599,
	}

	bodyShapes := []struct {
		name string
		data string
	}{
		{name: "empty body", data: ""},
		{name: "plain text", data: "plain text error body"},
		{name: "html page", data: "<html><body>Internal Server Error</body></html>"},
		{name: "valid errors envelope", data: `{"errors":[{"message":"upstream error"}]}`},
	}

	for _, status := range statuses {
		for _, shape := range bodyShapes {
			name := fmt.Sprintf("status_%d_%s", status, shape.name)
			t.Run(name, func(t *testing.T) {
				res := &http.Response{
					StatusCode: status,
					Body:       io.NopCloser(strings.NewReader(shape.data)),
					Header:     http.Header{},
				}

				var cfg *clicfg.Config
				var cred *credentials.AuraCredential
				if status == http.StatusUnauthorized || status == http.StatusForbidden {
					cfg = authCfg
					cred = authCred
				}

				var err error
				assert.NotPanics(t, func() {
					err = handleResponseError(res, cred, cfg)
				}, "handleResponseError panicked for status %d with %s", status, shape.name)

				require.Error(t, err, "handleResponseError returned nil error for status %d with %s", status, shape.name)
				var ce *clierr.CLIError
				require.True(t, errors.As(err, &ce), "errors.As should extract *CLIError; got %T: %v", err, err)
				_, ok := clierr.Codes[ce.Code]
				assert.True(t, ok, "CLIError.Code %d not in clierr.Codes for status %d with %s", ce.Code, status, shape.name)
			})
		}
	}
}
