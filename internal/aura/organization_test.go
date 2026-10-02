// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

const testCredJSON = `{
	"aura": {
		"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
		"default-credential": "c"
	}
}`

// newTestClient serves body with status at path (plus a token endpoint) and
// returns a Client wired to it, so the HTTP adapter is exercised end to end.
func newTestClient(t *testing.T, path string, status int, body string) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			w.Write([]byte(body)) //nolint:errcheck
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, srv.URL, srv.URL)
	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)
	return New(clicfg.NewConfig(fs, "test"))
}

func TestOrganizationsList(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErrStr string
		wantIDs    []string
	}{
		{name: "multiple", status: 200, body: `{"data":[{"id":"org-1","name":"One"},{"id":"org-2","name":"Two"}]}`, wantIDs: []string{"org-1", "org-2"}},
		{name: "empty", status: 200, body: `{"data":[]}`},
		{name: "404", status: 404, body: `{"errors":[{"message":"not found"}]}`, wantErrStr: "not found"},
		{name: "500", status: 500, body: `{"errors":[{"message":"internal server error"}]}`, wantErrStr: "internal server error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, "/v2beta1/organizations", tc.status, tc.body)

			orgs, err := c.Organizations().List(context.Background())
			if tc.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrStr)
				return
			}
			require.NoError(t, err)
			var ids []string
			for _, o := range orgs {
				ids = append(ids, o.ID)
				assert.NotEmpty(t, o.Record, "Record keeps the full server record")
			}
			assert.Equal(t, tc.wantIDs, ids)
		})
	}
}

func TestOrganizationsGet(t *testing.T) {
	const id = "org-abc-123"
	tests := []struct {
		name       string
		status     int
		body       string
		wantErrStr string
	}{
		{name: "success", status: 200, body: `{"data":{"id":"org-abc-123","name":"My Org","extra":"kept"}}`},
		{name: "404", status: 404, body: `{"errors":[{"message":"organization not found"}]}`, wantErrStr: "organization not found"},
		{name: "500", status: 500, body: `{"errors":[{"message":"server error"}]}`, wantErrStr: "server error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, "/v2beta1/organizations/"+id, tc.status, tc.body)

			org, err := c.Organizations().Get(context.Background(), id)
			if tc.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrStr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, id, org.ID)
			assert.Equal(t, "My Org", org.Name)
			assert.Equal(t, "kept", org.Record["extra"])
		})
	}

	t.Run("rejects an id that would retarget the path", func(t *testing.T) {
		_, err := organizationService{}.Get(context.Background(), "../x")
		var ce *clierr.CLIError
		assert.True(t, errors.As(err, &ce))
	})
}

func TestProjectsList(t *testing.T) {
	const orgID = "org-xyz-456"
	tests := []struct {
		name       string
		status     int
		body       string
		wantErrStr string
		wantLen    int
	}{
		{name: "multiple", status: 200, body: `{"data":[{"id":"proj-1","name":"One"},{"id":"proj-2","name":"Two"}]}`, wantLen: 2},
		{name: "empty", status: 200, body: `{"data":[]}`},
		{name: "404", status: 404, body: `{"errors":[{"message":"organization not found"}]}`, wantErrStr: "organization not found"},
		{name: "500", status: 500, body: `{"errors":[{"message":"internal server error"}]}`, wantErrStr: "internal server error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, "/v2beta1/organizations/"+orgID+"/projects", tc.status, tc.body)

			projects, err := c.Projects().List(context.Background(), orgID)
			if tc.wantErrStr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrStr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, projects, tc.wantLen)
		})
	}

	t.Run("rejects an empty or hostile org id", func(t *testing.T) {
		for _, bad := range []string{"", "..", "a/b"} {
			_, err := projectService{}.List(context.Background(), bad)
			assert.Error(t, err, bad)
		}
	})
}

func TestProjectsGet_DerivedFromList(t *testing.T) {
	const orgID = "org-1"
	body := `{"data":[{"id":"proj-1","name":"One"},{"id":"proj-2","name":"Two"}]}`

	t.Run("found", func(t *testing.T) {
		c := newTestClient(t, "/v2beta1/organizations/"+orgID+"/projects", 200, body)
		p, err := c.Projects().Get(context.Background(), orgID, "proj-2")
		require.NoError(t, err)
		assert.Equal(t, "Two", p.Name)
	})

	t.Run("absent project is a structured not-found", func(t *testing.T) {
		c := newTestClient(t, "/v2beta1/organizations/"+orgID+"/projects", 200, body)
		_, err := c.Projects().Get(context.Background(), orgID, "proj-9")

		var ce *clierr.CLIError
		require.True(t, errors.As(err, &ce))
		assert.Equal(t, 3, ce.Code)
		assert.Equal(t, "project", ce.ResourceType)
		assert.Equal(t, "proj-9", ce.ResourceID)
		assert.Contains(t, ce.Suggestion, "aura project list")
	})

	t.Run("upstream error is returned", func(t *testing.T) {
		c := newTestClient(t, "/v2beta1/organizations/"+orgID+"/projects", 500, `{"errors":[{"message":"boom"}]}`)
		_, err := c.Projects().Get(context.Background(), orgID, "proj-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})
}
