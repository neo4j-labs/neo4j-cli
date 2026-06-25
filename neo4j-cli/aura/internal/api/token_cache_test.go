// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envCfg builds a config with accept-env-vars enabled and an empty store, then
// installs an env-synthesized active credential (mirroring applyEnvCredential).
func envCfg(t *testing.T, serverURL, clientID, clientSecret string) *clicfg.Config {
	t.Helper()
	cfgJSON := fmt.Sprintf(`{
		"format": "json",
		"accept-env-vars": true,
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s"
		}
	}`, serverURL, serverURL)
	fs, err := testfs.GetTestFs(cfgJSON, `{"aura":{"credentials":[],"default-credential":""}}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cfg.Aura.SetActiveCredential(&credentials.AuraCredential{
		Name:         "env",
		ClientId:     clientID,
		ClientSecret: clientSecret,
	})
	return cfg
}

func doRequest(t *testing.T, cfg *clicfg.Config) {
	t.Helper()
	_, status, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
}

// instancesOnlyServer serves /v1/instances but NOT /oauth/token, so any actual
// HTTP mint would fail — proving mints are served by the seam/cache.
func instancesOnlyServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestTokenCache_EnvMode_SecondInvocationHitsCache(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	var mints int
	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		mints++
		return api.Grant{AccessToken: "minted-jwt", ExpiresIn: 3600}, nil
	})

	srv := instancesOnlyServer(t)

	doRequest(t, envCfg(t, srv, "id", "secret"))
	doRequest(t, envCfg(t, srv, "id", "secret"))

	assert.Equal(t, 1, mints, "second env invocation should hit the disk cache")
}

func TestTokenCache_EnvMode_SecretChangeMisses(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	var mints int
	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		mints++
		return api.Grant{AccessToken: "minted-jwt", ExpiresIn: 3600}, nil
	})

	srv := instancesOnlyServer(t)

	doRequest(t, envCfg(t, srv, "id", "secret-a"))
	doRequest(t, envCfg(t, srv, "id", "secret-b"))

	assert.Equal(t, 2, mints, "changing the client secret must miss the cache and re-mint")
}

func TestTokenCache_EnvMode_ExpiredMisses(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	var mints int
	// First mint returns a token that expires inside the 60s buffer window.
	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		mints++
		return api.Grant{AccessToken: "minted-jwt", ExpiresIn: 30}, nil
	})

	srv := instancesOnlyServer(t)

	doRequest(t, envCfg(t, srv, "id", "secret"))
	doRequest(t, envCfg(t, srv, "id", "secret"))

	assert.Equal(t, 2, mints, "a token within the 60s buffer must be treated as expired and re-minted")
}

func TestTokenCache_EnvMode_FileContainsOnlyJWTAt0600(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		return api.Grant{AccessToken: "minted-jwt", ExpiresIn: 3600}, nil
	})

	srv := instancesOnlyServer(t)
	const clientID, clientSecret = "id", "super-secret-value"
	doRequest(t, envCfg(t, srv, clientID, clientSecret))

	path := api.TokenCachePathForTest(clientID, clientSecret, srv+"/oauth/token")
	data, err := os.ReadFile(path) //nolint:gosec // test-controlled temp path
	require.NoError(t, err)

	assert.NotContains(t, string(data), clientSecret, "cache file must never contain the raw client secret")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(data, &entry))
	assert.Equal(t, "minted-jwt", entry["token"])
	assert.NotEmpty(t, entry["expiry"])
	assert.NotEmpty(t, entry["hash"])
	assert.NotContains(t, entry, "client-secret")

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestTokenCache_NotConsultedInStoredMode(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	var mints int
	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		mints++
		return api.Grant{AccessToken: "minted-jwt", ExpiresIn: 3600}, nil
	})

	// Stored credential (in the store, no valid token) — keyring/insecure mode.
	const storedCreds = `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"secret","access-token":"","token-expiry":0}],
			"default-credential": "c"
		}
	}`
	srv := instancesOnlyServer(t)
	cfg := buildTestConfig(t, srv, storedCreds)

	doRequest(t, cfg)

	assert.Equal(t, 1, mints)

	// No disk token cache must be written for a stored credential; it persists
	// via the store instead.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "stored-mode must not write a disk token cache")
}

func TestTokenCache_NotConsultedWhenAcceptEnvVarsOff(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	var mints int
	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		mints++
		return api.Grant{AccessToken: "minted-jwt", ExpiresIn: 3600}, nil
	})

	srv := instancesOnlyServer(t)
	// accept-env-vars off, but an active credential not in the store.
	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, srv, srv)
	fs, err := testfs.GetTestFs(cfgJSON, `{"aura":{"credentials":[],"default-credential":""}}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cfg.Aura.SetActiveCredential(&credentials.AuraCredential{Name: "env", ClientId: "id", ClientSecret: "secret"})

	doRequest(t, cfg)
	doRequest(t, cfg)

	assert.Equal(t, 2, mints, "cache must not be consulted when accept-env-vars is off")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// guards against any drift in the buffer constant's interaction with expiry.
func TestTokenCache_FreshTokenIsReused(t *testing.T) {
	dir := t.TempDir()
	api.SetTokenCacheDirForTest(t, dir)

	calls := 0
	api.SetMintTokenForTest(t, func() (api.Grant, error) {
		calls++
		// Comfortably beyond the buffer.
		return api.Grant{AccessToken: "jwt", ExpiresIn: int64((10 * time.Minute).Seconds())}, nil
	})

	srv := instancesOnlyServer(t)
	doRequest(t, envCfg(t, srv, "id", "s"))
	doRequest(t, envCfg(t, srv, "id", "s"))
	assert.Equal(t, 1, calls)
}
