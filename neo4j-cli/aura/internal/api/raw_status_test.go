// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRawStatusError_StatusMatrix covers every status the v2beta1 spec documents
// (REQ-F-027): 2xx is not an error, the modelled statuses keep the exit codes
// handleResponseError assigns, and the statuses that panic there — 413, 415,
// 422 — fall back on class: an unmapped 4xx is a permanent validation error, an
// unmapped 5xx is retryable. The envelope's retryable flag is asserted alongside
// the exit code because that is what an agent harness reads to decide on a retry.
func TestRawStatusError_StatusMatrix(t *testing.T) {
	for _, tc := range []struct {
		status        int
		body          string
		wantCode      int
		wantRetryable bool
	}{
		{status: http.StatusOK, body: `{"data":[]}`, wantCode: 0},
		{status: http.StatusCreated, body: `{"data":{"id":"a"}}`, wantCode: 0},
		{status: http.StatusAccepted, body: `{"data":{"id":"a"}}`, wantCode: 0},
		{status: http.StatusNoContent, body: ``, wantCode: 0},
		{status: http.StatusBadRequest, body: `{"errors":[{"message":"bad"}]}`, wantCode: 6},
		{status: http.StatusUnauthorized, body: `{"errors":[{"message":"nope"}]}`, wantCode: 4},
		{status: http.StatusForbidden, body: `{"error":"forbidden"}`, wantCode: 4},
		{status: http.StatusNotFound, body: `{"errors":[{"message":"gone"}]}`, wantCode: 3},
		{status: http.StatusMethodNotAllowed, body: ``, wantCode: 8, wantRetryable: true},
		{status: http.StatusPaymentRequired, body: `{"errors":[{"message":"quota"}]}`, wantCode: 5},
		{status: http.StatusConflict, body: `{"errors":[{"message":"busy"}]}`, wantCode: 5},
		{status: http.StatusRequestEntityTooLarge, body: ``, wantCode: 6},
		{status: http.StatusUnsupportedMediaType, body: ``, wantCode: 6},
		{status: http.StatusUnprocessableEntity, body: `{"errors":[{"message":"nope"}]}`, wantCode: 6},
		{status: http.StatusTeapot, body: ``, wantCode: 6},
		{status: http.StatusUnavailableForLegalReasons, body: ``, wantCode: 6},
		{status: http.StatusTooManyRequests, body: ``, wantCode: 7, wantRetryable: true},
		{status: http.StatusInternalServerError, body: `{"errors":[{"message":"boom"}]}`, wantCode: 8, wantRetryable: true},
		{status: http.StatusBadGateway, body: `<html>oops</html>`, wantCode: 8, wantRetryable: true},
		{status: http.StatusServiceUnavailable, body: ``, wantCode: 8, wantRetryable: true},
		{status: http.StatusGatewayTimeout, body: ``, wantCode: 8, wantRetryable: true},
		{status: http.StatusInsufficientStorage, body: ``, wantCode: 8, wantRetryable: true},
		{status: http.StatusPermanentRedirect, body: ``, wantCode: 8, wantRetryable: true},
	} {
		t.Run(fmt.Sprintf("%d", tc.status), func(t *testing.T) {
			res := &api.RawResponse{
				StatusCode: tc.status,
				Status:     fmt.Sprintf("%d %s", tc.status, http.StatusText(tc.status)),
				Body:       []byte(tc.body),
			}

			err := api.RawStatusError(res)
			if tc.wantCode == 0 {
				assert.NoError(t, err)
				return
			}

			ce := requireCLIErrorCode(t, err, tc.wantCode)
			assert.Contains(t, ce.Message, http.StatusText(tc.status))
			if tc.body != "" {
				assert.Contains(t, ce.Message, tc.body)
			}
			assert.Equal(t, tc.wantRetryable, ce.BuildEnvelope().Error.Retryable)
		})
	}
}

// TestRawStatusError_RateLimitCarriesRetryAfter asserts the 429 mapping keeps the
// Retry-After hint on the error so the envelope and the suggestion can surface it.
func TestRawStatusError_RateLimitCarriesRetryAfter(t *testing.T) {
	res := &api.RawResponse{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     http.Header{"Retry-After": {"30"}},
	}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 7)
	assert.Equal(t, "30", ce.RetryAfter)
	assert.Equal(t, "Retry after 30 seconds.", ce.Suggestion)
}

// TestRawStatusError_RateLimitWithoutRetryAfter asserts a 429 with no hint still
// maps cleanly rather than advertising an empty cool-off period.
func TestRawStatusError_RateLimitWithoutRetryAfter(t *testing.T) {
	res := &api.RawResponse{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests"}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 7)
	assert.Empty(t, ce.RetryAfter)
	assert.Empty(t, ce.Suggestion)
}

// TestRawStatusError_BodyShapes covers the bodies handleResponseError panics on:
// empty, non-JSON, and JSON that is not the api.Error shape. All must produce a
// clean error carrying the status.
func TestRawStatusError_BodyShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty", body: ``},
		{name: "plain text", body: `Bad Request`},
		{name: "html", body: `<html><body>nope</body></html>`},
		{name: "bare string", body: `"nope"`},
		{name: "null", body: `null`},
		{name: "unmodelled shape", body: `{"id":"e1","reason":"invalid","status_code":400}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := &api.RawResponse{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Body:       []byte(tc.body),
			}

			ce := requireCLIErrorCode(t, api.RawStatusError(res), 6)
			if tc.body == "" {
				assert.Equal(t, "aura api request failed with status 400 Bad Request", ce.Message)
				return
			}
			assert.Equal(t, "aura api request failed with status 400 Bad Request: "+tc.body, ce.Message)
		})
	}
}

// TestRawStatusError_MissingStatusLine asserts the status text is derived from the
// code when the response carries no status line.
func TestRawStatusError_MissingStatusLine(t *testing.T) {
	res := &api.RawResponse{StatusCode: http.StatusNotFound}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 3)
	assert.Contains(t, ce.Message, "404 Not Found")
}

// TestRawStatusError_UnknownStatusCode asserts a status with no http.StatusText
// still yields a clean upstream error rather than a dangling status line.
func TestRawStatusError_UnknownStatusCode(t *testing.T) {
	res := &api.RawResponse{StatusCode: 599, Body: []byte(`weird`)}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 8)
	assert.Contains(t, ce.Message, "status 599")
	assert.Contains(t, ce.Message, "weird")
}

// TestRawStatusError_ControlStripped asserts an ANSI escape cannot reach the
// terminal (or the tee file) through the error message, from either
// upstream-controlled half: net/http does not filter the reason phrase, and the
// body is whatever the endpoint returned.
func TestRawStatusError_ControlStripped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		body   string
	}{
		{name: "body", status: "409 Conflict", body: "\x1b[31mred\x1b[0m"},
		{name: "reason phrase", status: "409 \x1b[31mConflict\x1b[0m", body: "red"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := &api.RawResponse{
				StatusCode: http.StatusConflict,
				Status:     tc.status,
				Body:       []byte(tc.body),
			}

			ce := requireCLIErrorCode(t, api.RawStatusError(res), 5)
			assert.NotContains(t, ce.Message, "\x1b")
			assert.Contains(t, ce.Message, "Conflict")
			assert.Contains(t, ce.Message, "red")
		})
	}
}

// TestRawStatusError_BodyRedacted asserts a secret echoed back by the API does not
// survive into the error message, which is persisted by the tee-on-failure path.
func TestRawStatusError_BodyRedacted(t *testing.T) {
	res := &api.RawResponse{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       []byte(`{"errors":[{"message":"rejected","password":"hunter2"}]}`),
	}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 6)
	assert.NotContains(t, ce.Message, "hunter2")
	assert.Contains(t, ce.Message, "***")
}

// TestRawStatusError_BodyTruncated asserts a large body is bounded so the JSON
// error envelope on stdout stays readable. The oversized case also crosses the
// pre-scrub bound, which must not leave a second truncation marker behind. Both
// fallback branches are driven so neither loses the status line or the body.
func TestRawStatusError_BodyTruncated(t *testing.T) {
	for _, fallback := range []struct {
		status   int
		line     string
		wantCode int
	}{
		{status: http.StatusUnprocessableEntity, line: "422 Unprocessable Entity", wantCode: 6},
		{status: http.StatusInternalServerError, line: "500 Internal Server Error", wantCode: 8},
	} {
		for _, size := range []int{10_000, 500_000} {
			t.Run(fmt.Sprintf("%d/%d bytes", fallback.status, size), func(t *testing.T) {
				res := &api.RawResponse{
					StatusCode: fallback.status,
					Status:     fallback.line,
					Body:       []byte(strings.Repeat("x", size)),
				}

				ce := requireCLIErrorCode(t, api.RawStatusError(res), fallback.wantCode)
				assert.Contains(t, ce.Message, fallback.line)
				assert.Equal(t, 1, strings.Count(ce.Message, "(truncated)"))
				assert.Less(t, len(ce.Message), 4200)
			})
		}
	}
}

// TestRawStatusError_UnmappedClientErrorBodyScrubbed asserts the unmapped-4xx
// fallback keeps the same scrubbing as every mapped status, since a 422 body
// echoes back the submitted payload.
func TestRawStatusError_UnmappedClientErrorBodyScrubbed(t *testing.T) {
	res := &api.RawResponse{
		StatusCode: http.StatusUnprocessableEntity,
		Status:     "422 \x1b[31mUnprocessable Entity\x1b[0m",
		Body:       []byte(`{"errors":[{"message":"rejected","password":"hunter2"}]}`),
	}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 6)
	assert.NotContains(t, ce.Message, "hunter2")
	assert.NotContains(t, ce.Message, "\x1b")
	assert.Contains(t, ce.Message, "***")
	assert.Contains(t, ce.Message, "Unprocessable Entity")
}

// TestRawStatusError_MultibyteBodyTruncatedOnRuneBoundary asserts truncation never
// splits a rune, which would leave invalid UTF-8 in the JSON envelope. The rune is
// 3 bytes wide so the 4096-byte cut lands mid-rune (4096 % 3 == 1) and the
// back-up loop is genuinely exercised.
func TestRawStatusError_MultibyteBodyTruncatedOnRuneBoundary(t *testing.T) {
	res := &api.RawResponse{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
		Body:       []byte(strings.Repeat("→", 2000)),
	}

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 8)
	assert.True(t, utf8.ValidString(ce.Message), "truncated message must stay valid UTF-8")
	assert.Contains(t, ce.Message, "(truncated)")
	assert.Equal(t, 1365, strings.Count(ce.Message, "→"), "cut must land on the last whole rune before the limit")
}

// TestRawStatusError_NilResponse asserts the defensive path returns an error
// rather than dereferencing nil.
func TestRawStatusError_NilResponse(t *testing.T) {
	_ = requireCLIErrorCode(t, api.RawStatusError(nil), 8)
}

// TestRawStatusError_OnLiveResponse pairs the mapper with MakeRawRequest so the
// end-to-end contract — body preserved on a non-2xx, then mapped to an exit code —
// is covered against a real response rather than a hand-built struct.
func TestRawStatusError_OnLiveResponse(t *testing.T) {
	srv, _ := rawTestServer(t, http.StatusUnprocessableEntity, `{"errors":[{"message":"invalid region"}]}`)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodPost,
		VersionPath: "v2beta1",
		Path:        "instances/abc/databases",
		Body:        []byte(`{"name":"prod"}`),
	})
	require.NoError(t, err)

	ce := requireCLIErrorCode(t, api.RawStatusError(res), 6)
	assert.Contains(t, ce.Message, "invalid region")
}
