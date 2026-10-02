// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package transport_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const debugTestCredJSON = `{
	"aura": {
		"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"super-secret-token","token-expiry":99999999999999}],
		"default-credential": "c"
	}
}`

func debugTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"abc","name":"prod"}]}`)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMakeRequest_DebugEmitsWireAndTiming(t *testing.T) {
	srv := debugTestServer(t)
	var buf bytes.Buffer
	transport.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	cfg.AuraRuntime.SetDebug(true)

	body, status, err := transport.MakeRequest(context.Background(), cfg, "instances", &transport.RequestConfig{
		Method:   http.MethodPost,
		Version:  transport.AuraApiVersion1,
		PostBody: map[string]any{"name": "prod"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	out := buf.String()

	// Request line: method + URL.
	assert.Contains(t, out, "[aura-debug] > POST")
	assert.Contains(t, out, "/v1/instances")
	// Request headers emitted.
	assert.Contains(t, out, "Authorization:")
	// Request body emitted.
	assert.Contains(t, out, `"name":"prod"`)
	// Response line: status + body.
	assert.Contains(t, out, "[aura-debug] < 200")
	assert.Contains(t, out, `"id":"abc"`)
	// Duration emitted.
	assert.Contains(t, out, "elapsed")

	// Secret never leaks; Authorization redacted.
	assert.NotContains(t, out, "super-secret-token")
	assert.Contains(t, out, "***")

	// Sanity: stdout body is the unredacted real response.
	assert.Contains(t, string(body), "abc")
}

func TestMakeRequest_DebugOffEmitsNothing(t *testing.T) {
	srv := debugTestServer(t)
	var buf bytes.Buffer
	transport.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	// debug not set -> off

	_, _, err := transport.MakeRequest(context.Background(), cfg, "instances", &transport.RequestConfig{
		Method:  http.MethodGet,
		Version: transport.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
