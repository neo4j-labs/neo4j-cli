// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package context_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/context"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const testCredJSON = `{
	"aura": {
		"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
		"default-credential": "c"
	}
}`

// buildValidateTestConfig returns a *clicfg.Config wired to the given server URL. It also
// returns the raw config JSON string that was written to the in-memory FS, so tests can read
// back the persisted value via testfs.GetTestConfig after a Set call.
func buildValidateTestConfig(t *testing.T, serverURL string) *clicfg.Config {
	t.Helper()

	cfgJSON := fmt.Sprintf(`{
		"format": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s"
		}
	}`, serverURL, serverURL)

	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cfg.Aura.SetBetaEnabled(true)
	return cfg
}

// readPersistedDefaultContext reads the raw config file from the FS associated with cfg
// and returns the aura.default-context value as stored on disk. This is necessary because
// AuraConfig.Set writes to the filesystem but does not update viper's in-memory state.
func readPersistedDefaultContext(t *testing.T, cfg *clicfg.Config) string {
	t.Helper()
	raw, err := testfs.GetTestConfig(cfg.Aura.Fs())
	require.NoError(t, err)
	return gjson.Get(raw, "aura.default-context").String()
}

func buildValidateTestServer(t *testing.T, path string, status int, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	if path != "" {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			if body != "" {
				w.Write([]byte(body)) //nolint:errcheck
			}
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestValidateAndSetDefaultContext(t *testing.T) {
	const orgID = "org-abc-123"
	const projectID = "proj-def-456"
	const slug = orgID + "/" + projectID
	const projectPath = "/v2beta1/organizations/" + orgID + "/projects/" + projectID

	for _, tc := range []struct {
		name           string
		slug           string
		serverPath     string
		serverStatus   int
		serverBody     string
		wantErr        bool
		wantErrContain string
		wantContext    string
	}{
		{
			name:           "no slash in slug returns error",
			slug:           "noslug",
			wantErr:        true,
			wantErrContain: "expected format {organizationId}/{projectId}",
		},
		{
			name:           "empty org ID returns error",
			slug:           "/proj-123",
			wantErr:        true,
			wantErrContain: "organization ID must not be empty",
		},
		{
			name:           "empty project ID returns error",
			slug:           "org-123/",
			wantErr:        true,
			wantErrContain: "project ID must not be empty",
		},
		{
			name:           "404 from API returns error without persisting",
			slug:           slug,
			serverPath:     projectPath,
			serverStatus:   http.StatusNotFound,
			serverBody:     `{"errors": [{"message": "project not found"}]}`,
			wantErr:        true,
			wantErrContain: "project not found",
		},
		{
			name:           "500 from API returns error without persisting",
			slug:           slug,
			serverPath:     projectPath,
			serverStatus:   http.StatusInternalServerError,
			serverBody:     `{"errors": [{"message": "internal server error"}]}`,
			wantErr:        true,
			wantErrContain: "internal server error",
		},
		{
			name:         "success persists aura.default-context",
			slug:         slug,
			serverPath:   projectPath,
			serverStatus: http.StatusOK,
			serverBody:   fmt.Sprintf(`{"data": {"id": "%s", "name": "My Project"}}`, projectID),
			wantContext:  slug,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg *clicfg.Config
			if tc.serverPath != "" {
				srv := buildValidateTestServer(t, tc.serverPath, tc.serverStatus, tc.serverBody)
				cfg = buildValidateTestConfig(t, srv.URL)
			} else {
				// No server needed — error is in slug parsing before any API call.
				// Use a dummy URL; the request should never reach it.
				srv := buildValidateTestServer(t, "", 0, "")
				cfg = buildValidateTestConfig(t, srv.URL)
			}

			err := context.ValidateAndSetDefaultContext(cfg, tc.slug)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrContain != "" {
					assert.Contains(t, err.Error(), tc.wantErrContain)
				}
				// Verify nothing was persisted on error
				assert.Empty(t, readPersistedDefaultContext(t, cfg))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantContext, readPersistedDefaultContext(t, cfg))
		})
	}
}
