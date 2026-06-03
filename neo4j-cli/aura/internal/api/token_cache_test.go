// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gokeyring "github.com/zalando/go-keyring"
)

const (
	envClientID     = "env-client-id"
	envClientSecret = "env-client-secret"
)

// setupCountingServer is like setupServer but counts /oauth/token requests so a
// cache hit can be asserted to issue zero mints.
func setupCountingServer(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()

	var mintCount int32

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mintCount, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"minted-tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, &mintCount
}

// buildEnvModeConfig builds a config wired to serverURL, sets the Aura env vars
// so the "env" credential is synthesized from envClientID/envClientSecret, and
// switches cfg.Credentials into env storage mode.
func buildEnvModeConfig(t *testing.T, serverURL string) *clicfg.Config {
	t.Helper()

	// The credentials package reads env via os.Getenv; t.Setenv restores after.
	t.Setenv("NEO4J_AURA_CLIENT_ID", envClientID)
	t.Setenv("NEO4J_AURA_CLIENT_SECRET", envClientSecret)

	cfgJSON := fmt.Sprintf(`{
		"format": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s"
		}
	}`, serverURL, serverURL)

	fs, err := testfs.GetTestFs(cfgJSON, `{"aura":{"credentials":[]},"dbms":{"credentials":[]},"embed":{"credentials":[]}}`)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cfg.Credentials.SetStorageMode(credentials.StorageModeEnv, io.Discard)
	return cfg
}

// authURL returns the mint URL the env-mode config keys its cache identity on.
func authURL(serverURL string) string { return serverURL + "/oauth/token" }

// cachePathFor returns the fs path the cache uses for the env-mode identity.
func cachePathFor(serverURL string) string {
	identity := api.TokenIdentity(envClientID, envClientSecret, authURL(serverURL))
	return api.TokenCachePath(identity)
}

func makeInstancesRequest(t *testing.T, cfg *clicfg.Config) error {
	t.Helper()
	_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	return err
}

func TestTokenCache_MissMintsOnceAndWrites(t *testing.T) {
	srv, mintCount := setupCountingServer(t)
	cfg := buildEnvModeConfig(t, srv.URL)

	require.NoError(t, makeInstancesRequest(t, cfg))

	assert.Equal(t, int32(1), atomic.LoadInt32(mintCount), "cache miss mints exactly once")

	path := cachePathFor(srv.URL)
	exists, err := afero.Exists(cfg.Aura.Fs(), path)
	require.NoError(t, err)
	require.True(t, exists, "cache file written at the identity-keyed path")

	data, err := afero.ReadFile(cfg.Aura.Fs(), path)
	require.NoError(t, err)

	var cached struct {
		AccessToken string `json:"access-token"`
		TokenExpiry int64  `json:"token-expiry"`
		Identity    string `json:"identity"`
	}
	require.NoError(t, json.Unmarshal(data, &cached))
	assert.Equal(t, "minted-tok", cached.AccessToken)
	assert.Equal(t, api.TokenIdentity(envClientID, envClientSecret, authURL(srv.URL)), cached.Identity)
	assert.NotContains(t, string(data), envClientSecret, "raw secret never written to cache")
}

func TestTokenCache_HitSkipsMint(t *testing.T) {
	srv, mintCount := setupCountingServer(t)
	cfg := buildEnvModeConfig(t, srv.URL)

	seedCache(t, cfg.Aura.Fs(), cachePathFor(srv.URL), cacheEntry{
		AccessToken: "cached-tok",
		TokenExpiry: futureExpiry(),
		Identity:    api.TokenIdentity(envClientID, envClientSecret, authURL(srv.URL)),
	})

	require.NoError(t, makeInstancesRequest(t, cfg))

	assert.Equal(t, int32(0), atomic.LoadInt32(mintCount), "valid cache hit issues zero mints")
}

func TestTokenCache_FallsBackToMint(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry func(serverURL string) cacheEntry
		raw   []byte // when set, written verbatim instead of a marshaled entry
	}{
		{
			name: "identity mismatch (different secret folds into a different hash)",
			entry: func(serverURL string) cacheEntry {
				return cacheEntry{
					AccessToken: "cached-tok",
					TokenExpiry: futureExpiry(),
					Identity:    api.TokenIdentity(envClientID, "other-secret", authURL(serverURL)),
				}
			},
		},
		{
			name: "expired token",
			entry: func(serverURL string) cacheEntry {
				return cacheEntry{
					AccessToken: "cached-tok",
					TokenExpiry: time.Now().UnixMilli() - 1000,
					Identity:    api.TokenIdentity(envClientID, envClientSecret, authURL(serverURL)),
				}
			},
		},
		{
			name: "corrupt JSON",
			raw:  []byte("{not valid json"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, mintCount := setupCountingServer(t)
			cfg := buildEnvModeConfig(t, srv.URL)

			path := cachePathFor(srv.URL)
			if tc.raw != nil {
				require.NoError(t, afero.WriteFile(cfg.Aura.Fs(), path, tc.raw, 0600))
			} else {
				seedCache(t, cfg.Aura.Fs(), path, tc.entry(srv.URL))
			}

			require.NoError(t, makeInstancesRequest(t, cfg))

			assert.Equal(t, int32(1), atomic.LoadInt32(mintCount), "ignored cache falls back to a single mint")
		})
	}
}

func TestTokenCache_NonEnvModeNeverTouchesCache(t *testing.T) {
	for _, mode := range []string{credentials.StorageModeInsecure, credentials.StorageModeKeyring} {
		t.Run(mode, func(t *testing.T) {
			srv, mintCount := setupCountingServer(t)

			cfgJSON := fmt.Sprintf(`{
				"format": "json",
				"aura": {
					"auth-url": "%s/oauth/token",
					"base-url": "%s"
				}
			}`, srv.URL, srv.URL)
			credJSON := `{"aura":{"credentials":[{"name":"c","client-id":"id","client-secret":"s","access-token":"","token-expiry":0}],"default-credential":"c"},"dbms":{"credentials":[]},"embed":{"credentials":[]}}`
			// Keep the keyring branch hermetic: never touch the real OS keyring.
			gokeyring.MockInit()

			fs, err := testfs.GetTestFs(cfgJSON, credJSON)
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
			cfg.Credentials.SetStorageMode(mode, io.Discard)

			// A valid pre-seeded entry at the non-env identity path must NOT be read.
			path := api.TokenCachePath(api.TokenIdentity("id", "s", authURL(srv.URL)))
			seedCache(t, cfg.Aura.Fs(), path, cacheEntry{
				AccessToken: "cached-tok",
				TokenExpiry: futureExpiry(),
				Identity:    api.TokenIdentity("id", "s", authURL(srv.URL)),
			})

			require.NoError(t, makeInstancesRequest(t, cfg))

			assert.Equal(t, int32(1), atomic.LoadInt32(mintCount), "non-env mode never reads the temp cache")

			// The pre-seeded file is unchanged: the mint never wrote over it.
			data, err := afero.ReadFile(cfg.Aura.Fs(), path)
			require.NoError(t, err)
			assert.Contains(t, string(data), "cached-tok", "non-env mode never writes the temp cache")
		})
	}
}

type cacheEntry struct {
	AccessToken string `json:"access-token"`
	TokenExpiry int64  `json:"token-expiry"`
	Identity    string `json:"identity"`
}

func seedCache(t *testing.T, fs afero.Fs, path string, entry cacheEntry) {
	t.Helper()
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, path, data, 0600))
}

func futureExpiry() int64 { return time.Now().Add(time.Hour).UnixMilli() }
