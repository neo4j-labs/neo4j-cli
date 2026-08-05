// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedRequest records what the stub server actually received, so tests can
// assert on URL composition, headers, and the request body.
type capturedRequest struct {
	count  int
	method string
	path   string
	query  url.Values
	header http.Header
	body   string
}

// rawTestServer routes every path to one recording handler replying with the
// given status and body.
func rawTestServer(t *testing.T, status int, body string) (*httptest.Server, *capturedRequest) {
	t.Helper()

	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		got = capturedRequest{
			count:  got.count + 1,
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			header: r.Header.Clone(),
			body:   string(b),
		}
		w.WriteHeader(status)
		if body != "" {
			w.Write([]byte(body)) //nolint:errcheck
		}
	}))
	t.Cleanup(srv.Close)

	return srv, &got
}

func requireCLIErrorCode(t *testing.T, err error, code int) *clierr.CLIError {
	t.Helper()
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %v", err)
	assert.Equal(t, code, ce.Code)
	return ce
}

// TestMakeRawRequest_BodyPopulatedForEveryStatus locks the core passthrough
// contract: the upstream body survives on every status, non-2xx is not an error
// on this path, and no status or body shape panics.
func TestMakeRawRequest_BodyPopulatedForEveryStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{status: http.StatusOK, body: `{"data":[{"id":"abc"}]}`},
		{status: http.StatusCreated, body: `{"data":{"id":"abc"}}`},
		{status: http.StatusAccepted, body: `{"data":{"id":"abc"}}`},
		{status: http.StatusNoContent, body: ``},
		{status: http.StatusBadRequest, body: `{"errors":[{"message":"bad"}]}`},
		{status: http.StatusNotFound, body: `not json at all`},
		{status: http.StatusRequestEntityTooLarge, body: ``},
		{status: http.StatusUnsupportedMediaType, body: ``},
		{status: http.StatusUnprocessableEntity, body: `{"errors":[{"message":"nope"}]}`},
		{status: http.StatusTooManyRequests, body: ``},
		{status: http.StatusInternalServerError, body: `<html>oops</html>`},
	} {
		t.Run(fmt.Sprintf("%d", tc.status), func(t *testing.T) {
			srv, got := rawTestServer(t, tc.status, tc.body)
			cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

			res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
				Method:      http.MethodGet,
				VersionPath: "v2beta1",
				Path:        "instances",
			})
			require.NoError(t, err)
			require.NotNil(t, res)

			assert.Equal(t, tc.status, res.StatusCode)
			assert.Equal(t, tc.body, string(res.Body))
			assert.Contains(t, res.Status, fmt.Sprintf("%d", tc.status))
			assert.Equal(t, "HTTP/1.1", res.Proto)
			assert.NotNil(t, res.Header)
			assert.Equal(t, "/v2beta1/instances", got.path)
		})
	}
}

// TestMakeRawRequest_VersionPath covers the literal-version contract: an
// arbitrary segment is used verbatim with no enum lookup, and an empty
// VersionPath omits the segment entirely.
func TestMakeRawRequest_VersionPath(t *testing.T) {
	for _, tc := range []struct {
		name        string
		versionPath string
		path        string
		wantPath    string
	}{
		{name: "released version", versionPath: "v1", path: "instances", wantPath: "/v1/instances"},
		{name: "unreleased version", versionPath: "v9alpha3", path: "instances/abc", wantPath: "/v9alpha3/instances/abc"},
		{name: "no version segment", versionPath: "", path: "oauth/token", wantPath: "/oauth/token"},
		{name: "leading slash on path", versionPath: "v2beta1", path: "/instances", wantPath: "/v2beta1/instances"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := rawTestServer(t, http.StatusOK, `{"data":[]}`)
			cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

			res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
				Method:      http.MethodGet,
				VersionPath: tc.versionPath,
				Path:        tc.path,
			})
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Equal(t, tc.wantPath, got.path)
		})
	}
}

// TestMakeRawRequest_2xxWithEmbeddedErrorsIsSuccess is the counterpart to
// TestMakeRequest_2xxWithEmbeddedErrors: seven v2beta1 operations return
// {"data","errors"} on a genuine 200, so the passthrough must not convert that
// into a not-found error.
func TestMakeRawRequest_2xxWithEmbeddedErrors(t *testing.T) {
	const body = `{"data":{"id":"x"},"errors":[{"message":"DB not found: x","reason":"db-not-found"}]}`
	srv, _ := rawTestServer(t, http.StatusOK, body)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances/x",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, body, string(res.Body))
}

// TestMakeRawRequest_QueryParams asserts repeated keys survive — the raw config
// carries url.Values, unlike RequestConfig's map[string]string.
func TestMakeRawRequest_QueryParams(t *testing.T) {
	srv, got := rawTestServer(t, http.StatusOK, `{"data":[]}`)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	_, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
		QueryParams: url.Values{
			"page_token":      {"tok"},
			"database_status": {"online", "offline"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"tok"}, got.query["page_token"])
	assert.Equal(t, []string{"online", "offline"}, got.query["database_status"])
}

// TestMakeRawRequest_BodySentVerbatim covers the shape MakeRequest cannot
// express: a top-level JSON array, plus byte-for-byte fidelity of the payload.
func TestMakeRawRequest_BodySentVerbatim(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "object", body: `{"name":"prod","nested":{"a":[1,2]}}`},
		{name: "top-level array", body: `[{"id":"a"},{"id":"b"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := rawTestServer(t, http.StatusCreated, `{"data":{}}`)
			cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

			_, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
				Method:      http.MethodPost,
				VersionPath: "v2beta1",
				Path:        "databases",
				Body:        []byte(tc.body),
			})
			require.NoError(t, err)

			assert.Equal(t, http.MethodPost, got.method)
			assert.Equal(t, tc.body, got.body)
		})
	}
}

// TestMakeRawRequest_HeadersOverlaid asserts caller headers replace, rather than
// append to, the generated header of the same name (regardless of the case they
// are written in), while every generated header they don't name survives.
func TestMakeRawRequest_HeadersOverlaid(t *testing.T) {
	srv, got := rawTestServer(t, http.StatusOK, `{"data":[]}`)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	_, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
		Headers: http.Header{
			"content-type": {"application/yaml"},
			"Accept":       {"application/yaml"},
			"X-Custom":     {"one", "two"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"application/yaml"}, got.header.Values("Content-Type"))
	assert.Equal(t, "application/yaml", got.header.Get("Accept"))
	assert.Equal(t, []string{"one", "two"}, got.header.Values("X-Custom"))
	assert.Equal(t, "Bearer super-secret-token", got.header.Get("Authorization"))
	assert.Contains(t, got.header.Get("User-Agent"), "Neo4jCLI")
}

// TestMakeRawRequest_TransportFailure asserts a dial failure returns an
// upstream *CLIError rather than panicking the way MakeRequest does.
func TestMakeRawRequest_TransportFailure(t *testing.T) {
	srv, _ := rawTestServer(t, http.StatusOK, `{"data":[]}`)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	srv.Close()

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
	})
	assert.Nil(t, res)
	ce := requireCLIErrorCode(t, err, 8)
	assert.Contains(t, ce.Message, "aura api request failed")
}

// TestMakeRawRequest_BodyReadFailure asserts a truncated response (declared
// Content-Length longer than the bytes actually sent) surfaces as an upstream
// *CLIError instead of a panic from io.ReadAll.
func TestMakeRawRequest_BodyReadFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, buffered, err := hijacker.Hijack()
		require.NoError(t, err)
		_, err = buffered.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 500\r\n\r\ntruncated")
		require.NoError(t, err)
		require.NoError(t, buffered.Flush())
		require.NoError(t, conn.Close())
	}))
	t.Cleanup(srv.Close)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
	})
	assert.Nil(t, res)
	ce := requireCLIErrorCode(t, err, 8)
	assert.Contains(t, ce.Message, "could not read aura api response body")
}

// TestMakeRawRequest_401ClearsAccessToken asserts REQ-F-026: a 401 still runs
// the existing token-clearing path, so the next invocation mints a fresh token,
// and the response (with its body) is returned alongside the auth error.
func TestMakeRawRequest_401ClearsAccessToken(t *testing.T) {
	srv, _ := rawTestServer(t, http.StatusUnauthorized, `{"errors":[{"message":"unauthorized"}]}`)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
	})

	ce := requireCLIErrorCode(t, err, 4)
	assert.Contains(t, ce.Message, "unauthorized")
	assert.Contains(t, ce.Message, "access token has been cleared")

	require.NotNil(t, res)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.Equal(t, `{"errors":[{"message":"unauthorized"}]}`, string(res.Body))

	stored, err := testfs.GetTestCredentials(cfg.Aura.Fs())
	require.NoError(t, err)
	assert.NotContains(t, stored, "super-secret-token")
}

// TestMakeRawRequest_401EphemeralCredential covers the env-var-synthesised
// credential: it is absent from the store, so the 401 token clear cannot
// persist. The command must still fail cleanly with exit 4 and write nothing to
// credentials.json rather than panicking on the store lookup.
func TestMakeRawRequest_401EphemeralCredential(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errors":[{"message":"unauthorized"}]}`)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	const emptyCreds = `{"aura":{"credentials":[],"default-credential":""}}`
	cfg := buildTestConfig(t, srv.URL, emptyCreds)
	cfg.Aura.SetActiveCredential(&credentials.AuraCredential{
		Name:         "env",
		ClientId:     "env-client",
		ClientSecret: "env-secret",
	})

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
	})

	ce := requireCLIErrorCode(t, err, 4)
	assert.Contains(t, ce.Message, "unauthorized")
	require.NotNil(t, res)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)

	stored, err := testfs.GetTestCredentials(cfg.Aura.Fs())
	require.NoError(t, err)
	assert.Equal(t, emptyCreds, stored, "an ephemeral credential must never be persisted")
}

// TestMakeRawRequest_MethodRequired asserts a missing method is a usage error
// rather than the panic MakeRequest raises, and that no request is issued.
func TestMakeRawRequest_MethodRequired(t *testing.T) {
	srv, got := rawTestServer(t, http.StatusOK, `{"data":[]}`)
	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		VersionPath: "v1",
		Path:        "instances",
	})
	assert.Nil(t, res)
	ce := requireCLIErrorCode(t, err, 2)
	assert.Contains(t, ce.Message, "http method not set")
	assert.Zero(t, got.count, "no request may be issued without a method")
}

// TestMakeRawRequest_RejectsBlockedBaseURL asserts the raw entrypoint inherits
// MakeRequest's SSRF gate rather than opening a second unguarded path.
func TestMakeRawRequest_RejectsBlockedBaseURL(t *testing.T) {
	for _, baseURL := range []string{"http://169.254.169.254", "http://10.0.0.1", "ftp://example.com"} {
		t.Run(baseURL, func(t *testing.T) {
			cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"https://api.neo4j.io/oauth/token","base-url":"%s"}}`, baseURL)
			fs, err := testfs.GetTestFs(cfgJSON, debugTestCredJSON)
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

			res, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
				Method:      http.MethodGet,
				VersionPath: "v1",
				Path:        "instances",
			})
			assert.Nil(t, res)
			ce := requireCLIErrorCode(t, err, 2)
			assert.Contains(t, ce.Message, "aura base-url")
		})
	}
}

// TestMakeRawRequest_Debug asserts the passthrough traces through the same
// debugW seam and prefixes as MakeRequest, that the trace reflects the overlaid
// headers actually sent, and that the bearer token is redacted.
func TestMakeRawRequest_Debug(t *testing.T) {
	srv, _ := rawTestServer(t, http.StatusOK, `{"data":[{"id":"abc"}]}`)

	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	cfg.Aura.SetDebug(true)

	_, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodPost,
		VersionPath: "v2beta1",
		Path:        "databases",
		Body:        []byte(`{"name":"prod"}`),
		Headers:     http.Header{"Accept": {"application/yaml"}},
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "[aura-debug] > POST")
	assert.Contains(t, out, "/v2beta1/databases")
	assert.Contains(t, out, "Accept: application/yaml")
	assert.Contains(t, out, `{"name":"prod"}`)
	assert.Contains(t, out, "[aura-debug] < 200")
	assert.Contains(t, out, `"id":"abc"`)
	assert.Contains(t, out, "elapsed")
	assert.NotContains(t, out, "super-secret-token")
	assert.Contains(t, out, "***")
}

// TestMakeRawRequest_DebugOffEmitsNothing keeps the off-path untouched.
func TestMakeRawRequest_DebugOffEmitsNothing(t *testing.T) {
	srv, _ := rawTestServer(t, http.StatusOK, `{"data":[]}`)

	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)

	_, err := api.MakeRawRequest(cfg, &api.RawRequestConfig{
		Method:      http.MethodGet,
		VersionPath: "v1",
		Path:        "instances",
	})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
