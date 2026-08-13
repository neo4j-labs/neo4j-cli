// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCredJSON = `{
	"aura": {
		"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
		"default-credential": "c"
	}
}`

func buildOrgTestServer(t *testing.T, path string, status int, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			w.Write([]byte(body)) //nolint:errcheck
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListOrganizations(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		wantErr     bool
		wantErrStr  string
		wantOrgLen  int
		wantFirstID string
	}{
		{
			name:   "success with multiple organizations",
			status: http.StatusOK,
			body: `{
				"data": [
					{"id": "org-1", "name": "Org One"},
					{"id": "org-2", "name": "Org Two"}
				]
			}`,
			wantOrgLen:  2,
			wantFirstID: "org-1",
		},
		{
			name:       "success with empty organization list",
			status:     http.StatusOK,
			body:       `{"data": []}`,
			wantOrgLen: 0,
		},
		{
			name:       "404 returns error",
			status:     http.StatusNotFound,
			body:       `{"errors": [{"message": "not found"}]}`,
			wantErr:    true,
			wantErrStr: "not found",
		},
		{
			name:       "500 returns error",
			status:     http.StatusInternalServerError,
			body:       `{"errors": [{"message": "internal server error"}]}`,
			wantErr:    true,
			wantErrStr: "internal server error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildOrgTestServer(t, "/v2beta1/organizations", tc.status, tc.body)
			cfg := buildTestConfig(t, srv.URL, testCredJSON)

			resp, err := api.ListOrganizations(cfg)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrStr != "" {
					assert.Contains(t, err.Error(), tc.wantErrStr)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Len(t, resp.Data, tc.wantOrgLen)
			if tc.wantFirstID != "" && len(resp.Data) > 0 {
				assert.Equal(t, tc.wantFirstID, resp.Data[0].Id)
			}
		})
	}
}

func TestGetOrganization(t *testing.T) {
	orgID := "org-abc-123"

	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantErrStr string
		wantID     string
		wantName   string
	}{
		{
			name:   "success",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"data": {"id": "%s", "name": "My Org"}}`, orgID),
			wantID: orgID, wantName: "My Org",
		},
		{
			name:       "404 returns error",
			status:     http.StatusNotFound,
			body:       `{"errors": [{"message": "organization not found"}]}`,
			wantErr:    true,
			wantErrStr: "organization not found",
		},
		{
			name:       "500 returns error",
			status:     http.StatusInternalServerError,
			body:       `{"errors": [{"message": "server error"}]}`,
			wantErr:    true,
			wantErrStr: "server error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildOrgTestServer(t, fmt.Sprintf("/v2beta1/organizations/%s", orgID), tc.status, tc.body)
			cfg := buildTestConfig(t, srv.URL, testCredJSON)

			resp, err := api.GetOrganization(cfg, orgID)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrStr != "" {
					assert.Contains(t, err.Error(), tc.wantErrStr)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantID, resp.Data.Id)
			assert.Equal(t, tc.wantName, resp.Data.Name)
		})
	}
}

func TestListProjects(t *testing.T) {
	orgID := "org-xyz-456"

	for _, tc := range []struct {
		name           string
		status         int
		body           string
		wantErr        bool
		wantErrStr     string
		wantProjectLen int
		wantFirstID    string
	}{
		{
			name:   "success with multiple projects",
			status: http.StatusOK,
			body: `{
				"data": [
					{"id": "proj-1", "name": "Project One"},
					{"id": "proj-2", "name": "Project Two"}
				]
			}`,
			wantProjectLen: 2,
			wantFirstID:    "proj-1",
		},
		{
			name:           "success with empty project list",
			status:         http.StatusOK,
			body:           `{"data": []}`,
			wantProjectLen: 0,
		},
		{
			name:       "404 returns error",
			status:     http.StatusNotFound,
			body:       `{"errors": [{"message": "organization not found"}]}`,
			wantErr:    true,
			wantErrStr: "organization not found",
		},
		{
			name:       "500 returns error",
			status:     http.StatusInternalServerError,
			body:       `{"errors": [{"message": "internal server error"}]}`,
			wantErr:    true,
			wantErrStr: "internal server error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildOrgTestServer(t, fmt.Sprintf("/v2beta1/organizations/%s/projects", orgID), tc.status, tc.body)
			cfg := buildTestConfig(t, srv.URL, testCredJSON)

			resp, err := api.ListProjects(cfg, orgID)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrStr != "" {
					assert.Contains(t, err.Error(), tc.wantErrStr)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Len(t, resp.Data, tc.wantProjectLen)
			if tc.wantFirstID != "" && len(resp.Data) > 0 {
				assert.Equal(t, tc.wantFirstID, resp.Data[0].Id)
			}
		})
	}
}

func TestGetProject(t *testing.T) {
	orgID := "org-abc-123"
	projectID := "proj-def-456"

	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantErr    bool
		wantErrStr string
		wantID     string
		wantName   string
	}{
		{
			name:   "success",
			status: http.StatusOK,
			body:   fmt.Sprintf(`{"data": {"id": "%s", "name": "My Project"}}`, projectID),
			wantID: projectID, wantName: "My Project",
		},
		{
			name:       "404 returns error",
			status:     http.StatusNotFound,
			body:       `{"errors": [{"message": "project not found"}]}`,
			wantErr:    true,
			wantErrStr: "project not found",
		},
		{
			name:       "500 returns error",
			status:     http.StatusInternalServerError,
			body:       `{"errors": [{"message": "server error"}]}`,
			wantErr:    true,
			wantErrStr: "server error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildOrgTestServer(t, fmt.Sprintf("/v2beta1/organizations/%s/projects/%s", orgID, projectID), tc.status, tc.body)
			cfg := buildTestConfig(t, srv.URL, testCredJSON)

			resp, err := api.GetProject(cfg, orgID, projectID)

			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrStr != "" {
					assert.Contains(t, err.Error(), tc.wantErrStr)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.wantID, resp.Data.Id)
			assert.Equal(t, tc.wantName, resp.Data.Name)
		})
	}
}
