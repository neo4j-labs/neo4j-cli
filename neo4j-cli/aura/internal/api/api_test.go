// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
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
			// The active credential is fetched from the store by RegisterAuraCredentialFlag
			// (task-003) before SetActiveCredential is called, so it is always a pointer
			// into the store. Here we simulate that by including "active-cred" in the
			// credentials store and pointing SetActiveCredential at the same struct.
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
