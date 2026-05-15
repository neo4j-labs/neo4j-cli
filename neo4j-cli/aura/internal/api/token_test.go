// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetToken_DiskReread asserts that getToken returns the on-disk token
// without making an HTTP request to the token endpoint when another process
// has already refreshed the credential on disk.
//
// Setup:
//   - in-memory credential has an expired access token (token-expiry=0)
//   - credentials.json on the shared MemMapFs is overwritten with a fresh token
//     (expiry 1 hour in the future) AFTER the Config is constructed
//   - the /oauth/token handler records whether it was called
//
// Expected: getToken reads the fresh token from disk, syncs it into memory,
// and returns it without calling the token endpoint.
func TestGetToken_DiskReread(t *testing.T) {
	tokenEndpointCalled := false

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		tokenEndpointCalled = true
		// Return a valid token so MakeRequest does not panic if it somehow reaches here.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"http-tok","expires_in":3600}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Build a config whose in-memory credential has an expired token.
	expiredCredJSON := `{
		"aura": {
			"default-credential": "c",
			"credentials": [{"name":"c","client-id":"cid","client-secret":"sec","access-token":"expired","token-expiry":0}]
		}
	}`
	cfg := buildTestConfig(t, srv.URL, expiredCredJSON)

	// Overwrite credentials.json on the same MemMapFs with a fresh token so
	// ReloadAuraCredential can find it.
	freshExpiry := time.Now().Add(time.Hour).UnixMilli()
	freshCreds := map[string]any{
		"aura": map[string]any{
			"default-credential": "c",
			"credentials": []map[string]any{
				{
					"name":          "c",
					"client-id":     "cid",
					"client-secret": "sec",
					"access-token":  "fresh-tok",
					"token-expiry":  freshExpiry,
				},
			},
		},
	}
	data, err := json.Marshal(freshCreds)
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(cfg.Aura.Fs(), cfg.Credentials.FilePath(), data, 0o600))

	// MakeRequest drives getToken; the disk re-read should short-circuit before
	// any HTTP call to /oauth/token is made.
	_, _, err = api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)

	assert.False(t, tokenEndpointCalled, "token endpoint should NOT be called when disk has a fresh token")

	// Verify the in-memory credential was synced with the fresh disk token.
	cred, credErr := cfg.Credentials.Aura.Get("c")
	require.NoError(t, credErr)
	assert.Equal(t, "fresh-tok", cred.AccessToken, "in-memory token should be synced from disk")
	assert.Equal(t, freshExpiry, cred.TokenExpiry, "in-memory token expiry should be synced from disk")
}
