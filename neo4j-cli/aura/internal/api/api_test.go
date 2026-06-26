// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestConfig returns a minimal *clicfg.Config wired to the provided test server URL.
// credJSON is written to the credentials file, allowing control over which default credential is set.
func buildTestConfig(t *testing.T, serverURL string, credJSON string) *clicfg.Config {
	t.Helper()

	cfgJSON := fmt.Sprintf(`{
		"format": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s"
		}
	}`, serverURL, serverURL)

	fs, err := testfs.GetTestFs(cfgJSON, credJSON)
	require.NoError(t, err)

	return clicfg.NewConfig(fs, "test", clicfg.AuraScope)
}

// setupServer returns a test HTTP server that:
//   - records the Basic-Auth client-id used on /oauth/token requests
//   - responds with a valid token on /oauth/token
//   - returns 200 + empty data on /v1/instances
func setupServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()

	var capturedClientID string

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		clientID, _, _ := r.BasicAuth()
		capturedClientID = clientID
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv, &capturedClientID
}

func TestMakeRequest_CredentialResolution(t *testing.T) {
	for _, tc := range []struct {
		name         string
		credJSON     string
		setActive    *credentials.AuraCredential
		wantClientID string
		wantErr      bool
	}{
		{
			name: "falls back to default credential when ActiveCredential is nil",
			credJSON: `{
				"aura": {
					"credentials": [{"name":"default-cred","client-id":"default-client","client-secret":"secret","access-token":"","token-expiry":0}],
					"default-credential": "default-cred"
				}
			}`,
			setActive:    nil,
			wantClientID: "default-client",
		},
		{
			name: "uses ActiveCredential when set, bypassing GetDefault",
			credJSON: `{
				"aura": {
					"credentials": [
						{"name":"default-cred","client-id":"default-client","client-secret":"secret","access-token":"","token-expiry":0},
						{"name":"active-cred","client-id":"active-client","client-secret":"active-secret","access-token":"","token-expiry":0}
					],
					"default-credential": "default-cred"
				}
			}`,
			setActive: &credentials.AuraCredential{
				Name:         "active-cred",
				ClientId:     "active-client",
				ClientSecret: "active-secret",
			},
			wantClientID: "active-client",
		},
		{
			name: "returns error when ActiveCredential is nil and no default is set",
			credJSON: `{
				"aura": {
					"credentials": [],
					"default-credential": ""
				}
			}`,
			setActive: nil,
			wantErr:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, capturedClientID := setupServer(t)

			cfg := buildTestConfig(t, srv.URL, tc.credJSON)
			if tc.setActive != nil {
				cfg.Aura.SetActiveCredential(tc.setActive)
			}

			_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion1,
			})

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantClientID, *capturedClientID)
		})
	}
}

// TestMakeRequest_EnvCredentialNotPersisted drives a full request with an
// active credential that is NOT in the store (mirroring accept-env-vars
// synthesis). It must succeed (no panic from the UpdateAccessToken store
// lookup) and leave credentials.json untouched.
func TestMakeRequest_EnvCredentialNotPersisted(t *testing.T) {
	srv, capturedClientID := setupServer(t)

	const emptyCreds = `{"aura":{"credentials":[],"default-credential":""}}`
	cfg := buildTestConfig(t, srv.URL, emptyCreds)
	cfg.Aura.SetActiveCredential(&credentials.AuraCredential{
		Name:         "env",
		ClientId:     "env-client",
		ClientSecret: "env-secret",
	})

	before, err := testfs.GetTestCredentials(cfg.Aura.Fs())
	require.NoError(t, err)

	_, _, err = api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.NoError(t, err)
	assert.Equal(t, "env-client", *capturedClientID)

	after, err := testfs.GetTestCredentials(cfg.Aura.Fs())
	require.NoError(t, err)
	assert.Equal(t, before, after, "env-synthesized token must not be persisted to credentials.json")
}

// TestMakeRequest_Timeout asserts the http.Client timeout fires when the
// server stalls past the configured cap. Uses the test seam to dial the cap
// down to milliseconds so the assertion runs in <1s; production keeps the 60s
// cap. MakeRequest panics on client.Do errors today, so we recover and inspect
// the deadline-exceeded marker on the panic value.
func TestMakeRequest_Timeout(t *testing.T) {
	api.SetHTTPClientTimeoutForTest(t, 50*time.Millisecond)

	stall := make(chan struct{})
	t.Cleanup(func() { close(stall) })

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances", func(w http.ResponseWriter, r *http.Request) {
		// Block past the client timeout. Safety: bail at 5s so a propagation
		// regression doesn't hang the test goroutine indefinitely.
		select {
		case <-stall:
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := buildTestConfig(t, srv.URL, `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
			"default-credential": "c"
		}
	}`)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _, _ = api.MakeRequest(cfg, "instances", &api.RequestConfig{
			Method:  http.MethodGet,
			Version: api.AuraApiVersion1,
		})
	}()

	require.NotNil(t, recovered, "expected panic on timeout")
	msg := fmt.Sprintf("%v", recovered)
	assert.True(t,
		strings.Contains(msg, "deadline exceeded") ||
			strings.Contains(msg, "Client.Timeout") ||
			strings.Contains(msg, "context deadline exceeded"),
		"expected deadline-exceeded marker, got: %s", msg)
}

// TestMakeRequest_RejectsBlockedBaseURL asserts that MakeRequest rejects an
// SSRF-blocked aura base-url (e.g. cloud metadata IP) before issuing any HTTP
// traffic. The captured roundtripper would panic if hit.
func TestMakeRequest_RejectsBlockedBaseURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "metadata IP", baseURL: "http://169.254.169.254"},
		{name: "private RFC1918", baseURL: "http://10.0.0.1"},
		{name: "link-local", baseURL: "http://[fe80::1]"},
		{name: "cleartext non-loopback", baseURL: "http://api.openai.com"},
		{name: "malformed scheme", baseURL: "ftp://example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfgJSON := fmt.Sprintf(`{
				"format": "json",
				"aura": {
					"auth-url": "https://api.neo4j.io/oauth/token",
					"base-url": "%s"
				}
			}`, tc.baseURL)
			fs, err := testfs.GetTestFs(cfgJSON, `{
				"aura": {
					"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
					"default-credential": "c"
				}
			}`)
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

			_, _, err = api.MakeRequest(cfg, "instances", &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion1,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "aura base-url")
		})
	}
}

// TestMakeRequest_RejectsBlockedAuthURL asserts that getToken rejects an
// SSRF-blocked aura auth-url. The credential intentionally has no valid access
// token so the code path enters getToken.
func TestMakeRequest_RejectsBlockedAuthURL(t *testing.T) {
	cfgJSON := `{
		"format": "json",
		"aura": {
			"auth-url": "http://169.254.169.254/oauth/token",
			"base-url": "https://api.neo4j.io"
		}
	}`
	fs, err := testfs.GetTestFs(cfgJSON, `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"","token-expiry":0}],
			"default-credential": "c"
		}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	_, _, err = api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aura auth-url")
}

// TestMakeRequest_2xxWithEmbeddedErrors covers the latent path where the Aura
// API returns 200 OK with an `errors[]` array embedded in the response body
// (e.g. `{"data":{"id":"x"},"errors":[{"message":"DB not found: x"}]}`).
// MakeRequest must detect the embedded errors, surface them through
// NewNotFoundError with multi-line bracket Message shape, and populate
// ResourceType/ResourceID/Suggestion from the request path.
func TestMakeRequest_2xxWithEmbeddedErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v1/instances/x", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"id":"x"},"errors":[{"message":"DB not found: x","reason":"db-not-found"}]}`)) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := buildTestConfig(t, srv.URL, `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
			"default-credential": "c"
		}
	}`)

	_, _, err := api.MakeRequest(cfg, "instances/x", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Equal(t, "[\n\tDB not found: x\n]", ce.Message)
	assert.Equal(t, "instance", ce.ResourceType)
	assert.Equal(t, "x", ce.ResourceID)
	assert.Equal(t, "Run 'neo4j-cli aura instance list' to see available instances.", ce.Suggestion)
}

// TestGetToken_401_AuthError locks the task-004 reclassification: an aura
// token-endpoint 401 (invalid/expired/revoked client credentials) surfaces as
// *CLIError with Code == 4 (auth), not the previous usage exit.
func TestGetToken_401_AuthError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := buildTestConfig(t, srv.URL, `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"","token-expiry":0}],
			"default-credential": "c"
		}
	}`)

	_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code)
	assert.Contains(t, ce.Error(), "invalid, expired, or revoked")
}

// mintStatusConfig wires a config whose auth endpoint returns the given status
// and body, with a stored default credential so MakeRequest enters getToken.
func mintStatusConfig(t *testing.T, status int, body string) *clicfg.Config {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			w.Write([]byte(body)) //nolint:errcheck
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return buildTestConfig(t, srv.URL, `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"","token-expiry":0}],
			"default-credential": "c"
		}
	}`)
}

// TestGetToken_403_ForbiddenError locks REQ-F-019: a 403 from the token
// endpoint surfaces as a distinct "forbidden / not authorized" auth error — not
// the 401 wording — and never an empty-token success.
func TestGetToken_403_ForbiddenError(t *testing.T) {
	cfg := mintStatusConfig(t, http.StatusForbidden, "")

	_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 4, ce.Code)
	assert.Contains(t, ce.Error(), "forbidden")
	assert.Contains(t, ce.Error(), "not authorized")
	assert.NotContains(t, ce.Error(), "invalid, expired, or revoked")
	assert.NotContains(t, ce.Error(), "environment")
}

// TestGetToken_OtherNon2xx_NamesStatus locks REQ-F-019: an unlisted non-2xx
// (e.g. 422, 500) returns a clear error naming the status rather than the
// generic "please report" panic or an empty-token success.
func TestGetToken_OtherNon2xx_NamesStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			cfg := mintStatusConfig(t, status, "")

			_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion1,
			})
			require.Error(t, err)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Contains(t, ce.Error(), fmt.Sprintf("%d", status))
			assert.NotContains(t, ce.Error(), "please report")
		})
	}
}

// TestGetToken_EmptyTokenIsError locks REQ-F-019: a 200 with an empty
// access_token must not be returned as success (no "Authorization: Bearer ").
func TestGetToken_EmptyTokenIsError(t *testing.T) {
	cfg := mintStatusConfig(t, http.StatusOK, `{"access_token":"","expires_in":3600,"token_type":"bearer"}`)

	_, _, err := api.MakeRequest(cfg, "instances", &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion1,
	})
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Error(), "empty token")
	assert.NotContains(t, ce.Error(), "please report")
}
