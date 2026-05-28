// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoJSONRequest_HappyPathBodyCap asserts the helper caps a successful
// response body read at maxResponseBodyBytes so a compromised proxy or
// attacker-controlled BaseURL returning a multi-GB 200 OK cannot exhaust
// memory. Writing >cap bytes that aren't valid JSON forces a downstream
// decode failure (the truncation point lands inside a run of zero bytes);
// the read itself is bounded.
func TestDoJSONRequest_HappyPathBodyCap(t *testing.T) {
	const oversize = maxResponseBodyBytes + (1 << 20) // cap + 1 MiB

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Stream zero bytes well past the cap. If the helper were unbounded
		// the test process would buffer the full payload into memory; with
		// the cap in place it stops at maxResponseBodyBytes and the decode
		// below fails fast.
		buf := make([]byte, 64<<10)
		written := 0
		for written < oversize {
			n, err := w.Write(buf)
			if err != nil {
				return
			}
			written += n
		}
	}))
	defer srv.Close()

	raw, err := doJSONRequest(
		context.Background(),
		srv.Client(),
		"test",
		http.MethodPost,
		srv.URL,
		map[string]any{"k": "v"},
		nil,
		"neo4j-cli/test",
	)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(raw), maxResponseBodyBytes, "happy-path read must not exceed the cap")
	assert.Equal(t, maxResponseBodyBytes, len(raw), "expected exactly cap bytes when server writes past it")

	// The truncated payload isn't valid JSON, so the caller's decode step
	// surfaces a decode error rather than an OOM — verify that downstream
	// failure mode here.
	var out struct{}
	decodeErr := json.NewDecoder(strings.NewReader(string(raw))).Decode(&out)
	require.Error(t, decodeErr)
}

// TestRedactSensitiveURLParams covers each known-sensitive query parameter
// in isolation and confirms case-insensitive matching, multi-param URLs,
// position (first / last / middle), and the nil-passthrough.
func TestRedactSensitiveURLParams(t *testing.T) {
	const secret = "AIzaSyA-VERYSECRETVALUE12345"
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{
			name: "lowercase key as only param",
			in:   `Post "https://api.example.com/v1?key=` + secret + `": dial tcp: ...`,
			want: `Post "https://api.example.com/v1?key=REDACTED": dial tcp: ...`,
		},
		{
			name: "uppercase KEY",
			in:   `Post "https://api.example.com/v1?KEY=` + secret + `": dial tcp: ...`,
			want: `Post "https://api.example.com/v1?KEY=REDACTED": dial tcp: ...`,
		},
		{
			name: "api_key with leading ampersand",
			in:   `Post "https://api.example.com/v1?foo=bar&api_key=` + secret + `": eof`,
			want: `Post "https://api.example.com/v1?foo=bar&api_key=REDACTED": eof`,
		},
		{
			name: "mixed-case Api_Key",
			in:   `Post "https://api.example.com/v1?Api_Key=` + secret + `": eof`,
			want: `Post "https://api.example.com/v1?Api_Key=REDACTED": eof`,
		},
		{
			name: "access_token as first param",
			in:   `Post "https://api.example.com/v1?access_token=` + secret + `&other=1": eof`,
			want: `Post "https://api.example.com/v1?access_token=REDACTED&other=1": eof`,
		},
		{
			name: "Access_Token mixed case",
			in:   `Post "https://api.example.com/v1?Access_Token=` + secret + `": eof`,
			want: `Post "https://api.example.com/v1?Access_Token=REDACTED": eof`,
		},
		{
			name: "token alone",
			in:   `Post "https://api.example.com/v1?token=` + secret + `": eof`,
			want: `Post "https://api.example.com/v1?token=REDACTED": eof`,
		},
		{
			name: "TOKEN uppercase between two other params",
			in:   `Post "https://api.example.com/v1?a=1&TOKEN=` + secret + `&b=2": eof`,
			want: `Post "https://api.example.com/v1?a=1&TOKEN=REDACTED&b=2": eof`,
		},
		{
			name: "multiple sensitive params in one URL",
			in:   `Post "https://api.example.com/v1?key=abc&token=` + secret + `": eof`,
			want: `Post "https://api.example.com/v1?key=REDACTED&token=REDACTED": eof`,
		},
		{
			name: "non-sensitive param untouched",
			in:   `Post "https://api.example.com/v1?model=text-embedding-004": eof`,
			want: `Post "https://api.example.com/v1?model=text-embedding-004": eof`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSensitiveURLParams(errors.New(tc.in))
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got.Error())
			assert.NotContains(t, got.Error(), secret, "secret must never appear in the redacted error")
		})
	}

	t.Run("nil passthrough", func(t *testing.T) {
		assert.Nil(t, redactSensitiveURLParams(nil))
	})
}

// TestDoJSONRequest_NetworkErrorRedactsKey points the helper at an httptest
// URL with a sensitive query param, closes the server immediately so the
// next call returns a network error, and confirms the wrapped error masks
// the secret while still revealing the call site.
func TestDoJSONRequest_NetworkErrorRedactsKey(t *testing.T) {
	const secret = "AIzaSyA-VERYSECRETVALUE12345"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := srv.URL + "/v1/models/m:embedContent?key=" + secret
	srv.Close()

	_, err := doJSONRequest(
		context.Background(),
		srv.Client(),
		"gemini",
		http.MethodPost,
		target,
		map[string]any{"k": "v"},
		nil,
		"neo4j-cli/test",
	)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "REDACTED", "expected REDACTED in error, got: %s", msg)
	assert.NotContains(t, msg, secret, "secret leaked into error: %s", msg)
	assert.Contains(t, msg, "gemini: request failed:", "provider+stage prefix should survive redaction")
}

// TestRedactSensitiveURLParams_BuildRequestErrorShape confirms a synthetic
// build-request-style error string (the wording http.NewRequestWithContext
// could produce on URLs it rejects) runs through the redactor cleanly. The
// `build request:` wrap site is defensive: in practice urlcheck.ValidateRemoteURL
// rejects most URLs that NewRequestWithContext would also fail, so triggering
// the live path without breaking that ordering is brittle. Asserting the
// redactor handles the shape keeps the property covered without coupling the
// test to net/http internals.
func TestRedactSensitiveURLParams_BuildRequestErrorShape(t *testing.T) {
	const secret = "AIzaSyA-VERYSECRETVALUE12345"
	in := errors.New(`gemini: build request: parse "https://api.example.com/v1?key=` + secret + `": net/http: invalid request`)
	got := redactSensitiveURLParams(in)
	require.NotNil(t, got)
	assert.Contains(t, got.Error(), "REDACTED")
	assert.NotContains(t, got.Error(), secret)
	assert.Contains(t, got.Error(), "gemini: build request:")
}

// Sanity: ensure the fmt.Errorf at the wrap sites is still using %w so a
// non-redacted callsite would have unwrapped. After redaction the chain
// flattens (intentional trade-off), so callers cannot errors.Is/As through;
// we encode that invariant here so a future refactor doesn't silently
// reintroduce a non-flattened path that breaks the security property.
func TestRedactSensitiveURLParams_FlattensChain(t *testing.T) {
	inner := errors.New(`Post "https://x?key=abc": dial`)
	wrapped := fmt.Errorf("gemini: request failed: %w", inner)
	red := redactSensitiveURLParams(wrapped)
	require.Error(t, red)
	assert.False(t, errors.Is(red, inner), "redaction must flatten the chain — original sentinel must not be reachable")
}

// TestDoJSONRequest_HappyPathUnderCap confirms the cap is a ceiling, not a
// floor: a normal-sized response is read in full.
func TestDoJSONRequest_HappyPathUnderCap(t *testing.T) {
	body := []byte(`{"ok":true}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	raw, err := doJSONRequest(
		context.Background(),
		srv.Client(),
		"test",
		http.MethodPost,
		srv.URL,
		map[string]any{"k": "v"},
		nil,
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, body, raw)
}
