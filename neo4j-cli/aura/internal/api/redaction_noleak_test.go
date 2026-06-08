// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDebug_NoSecretLeaksAcrossPaths is the redaction safety boundary lock: it
// drives a request through the token path (fresh fetch + basic auth), the
// request path (Authorization: Bearer header + a secret-bearing request body),
// and the response path (a body echoing secret-shaped fields), then asserts no
// raw secret substring survives in the captured debug output. Redaction is the
// single guarantee that --debug never exposes credentials; if any of these leak
// the test fails loudly rather than silently shipping a secret to stderr.
func TestDebug_NoSecretLeaksAcrossPaths(t *testing.T) {
	const (
		clientSecret = "leak-canary-client-secret"
		grantToken   = "leak-canary-grant-access-token"
		bodyPassword = "leak-canary-body-password"
		bodySecret   = "leak-canary-body-secret"
		bearerToken  = "leak-canary-bearer-token" // unused on fresh fetch, asserted absent regardless
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		// Basic-auth carries client id/secret; ensure neither leaks even though
		// the token request body/headers are not themselves debug-dumped.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"` + grantToken + `","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Response body echoes secret-shaped fields the wire dump must scrub.
		w.Write([]byte(`{"data":{"password":"` + bodySecret + `","token":"` + grantToken + `"}}`)) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// noTokenCredJSON forces a fresh fetch (token path) using client-secret.
	credJSON := `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"` + clientSecret + `"}],
			"default-credential": "c"
		}
	}`

	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, credJSON)
	cfg.Aura.SetDebug(true)

	_, status, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:   http.MethodPost,
		Version:  api.AuraApiVersion1,
		PostBody: map[string]any{"password": bodyPassword, "name": "prod"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	out := buf.String()

	// Prove debug actually ran across all three paths (not a silent no-op).
	assert.Contains(t, out, "fetching new one", "token path did not emit")
	assert.Contains(t, out, "[aura-debug] > POST", "request path did not emit")
	assert.Contains(t, out, "[aura-debug] < 200", "response path did not emit")
	assert.Contains(t, out, "***", "redaction placeholder absent — nothing was scrubbed")

	// No raw secret survives anywhere in captured debug output.
	for _, secret := range []string{clientSecret, grantToken, bodyPassword, bodySecret, bearerToken} {
		assert.NotContainsf(t, out, secret, "raw secret %q leaked in debug output", secret)
	}

	// The Authorization header line is present but its value is scrubbed.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), "authorization") {
			assert.NotContains(t, line, grantToken, "Authorization header leaked the bearer token")
			assert.NotContains(t, line, "Bearer "+grantToken)
		}
	}
}
