// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credential with a valid (far-future) access token -> getToken reuses cache.
const cachedTokenCredJSON = `{
	"aura": {
		"credentials": [{"name":"c","client-id":"id","client-secret":"super-secret-client-secret","access-token":"super-secret-token","token-expiry":99999999999999}],
		"default-credential": "c"
	}
}`

// credential with no access token -> getToken fetches a new one.
const noTokenCredJSON = `{
	"aura": {
		"credentials": [{"name":"c","client-id":"id","client-secret":"super-secret-client-secret"}],
		"default-credential": "c"
	}
}`

func TestGetToken_DebugReportsCachedReuse(t *testing.T) {
	srv := debugTestServer(t)
	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, cachedTokenCredJSON)
	cfg.Aura.SetDebug(true)

	_, status, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	out := buf.String()
	assert.Contains(t, out, "reusing cached valid access token")
	// No secrets leak.
	assert.NotContains(t, out, "super-secret-token")
	assert.NotContains(t, out, "super-secret-client-secret")
}

func TestGetToken_DebugReportsFetchAndStatus(t *testing.T) {
	srv, _ := setupServer(t)
	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, noTokenCredJSON)
	cfg.Aura.SetDebug(true)

	_, status, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	out := buf.String()
	// Fetch decision + auth URL.
	assert.Contains(t, out, "fetching new one")
	assert.Contains(t, out, "/oauth/token")
	// Response status of the token request.
	assert.Contains(t, out, "auth response status 200")
	// Neither the client secret nor the returned access token leaks.
	assert.NotContains(t, out, "super-secret-client-secret")
	assert.NotContains(t, out, "\"tok\"")
}

func TestGetToken_DebugOffEmitsNoTokenLines(t *testing.T) {
	srv, _ := setupServer(t)
	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, noTokenCredJSON)
	// debug not set -> off

	_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
