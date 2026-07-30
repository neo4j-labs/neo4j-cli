// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOrgID     = "org-abc-123"
	testProjectID = "proj-def-456"
)

// listProjectsPath is the v2beta1 path cobra test servers must handle.
const listProjectsPath = "/v2beta1/organizations/" + testOrgID + "/projects"

// listProjectsSuccessBody is a response body that includes testProjectID.
const listProjectsSuccessBody = `{"data": [{"id": "` + testProjectID + `", "name": "My Project"}]}`

// listProjectsEmptyBody is a response body with no projects.
const listProjectsEmptyBody = `{"data": []}`

// buildTestServer creates an httptest.Server that:
//   - responds to /oauth/token with a dummy token
//   - responds to projectsPath (if non-empty) with the given status and body
func buildTestServer(t *testing.T, projectsPath string, status int, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	if projectsPath != "" {
		mux.HandleFunc(projectsPath, func(w http.ResponseWriter, r *http.Request) {
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

// buildTestConfig creates a *clicfg.Config pointing at the given server URL.
// extraCfg is an optional JSON fragment merged into the aura object (e.g.
// `"default-workspace": "org/proj"`).
func buildTestConfig(t *testing.T, serverURL, extraCfg string) *clicfg.Config {
	t.Helper()

	cfgJSON := fmt.Sprintf(`{
		"format": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s"
			%s
		}
	}`, serverURL, serverURL, extraCfg)

	credJSON := `{
		"aura": {
			"credentials": [{"name":"c","client-id":"id","client-secret":"s","access-token":"tok","token-expiry":9999999999}],
			"default-credential": "c"
		}
	}`

	fs, err := testfs.GetTestFs(cfgJSON, credJSON)
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	return cfg
}

// newTestCmd creates a cobra.Command with org/project flags registered (as
// persistent) and parses the given args. It does NOT execute a RunE.
func newTestCmd(t *testing.T, args []string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{
		Use:  "test",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	flags.RegisterOrgProjectFlags(cmd)
	cmd.SetArgs(args)
	// Execute so cobra parses the args and marks Changed() flags.
	require.NoError(t, cmd.Execute())
	return cmd
}

func TestResolveAndValidateOrgProject_OrgFromFlag(t *testing.T) {
	srv := buildTestServer(t, listProjectsPath, http.StatusOK, listProjectsSuccessBody)
	cfg := buildTestConfig(t, srv.URL, "")

	cmd := newTestCmd(t, []string{"--organization-id", testOrgID, "--project-id", testProjectID})

	gotOrg, gotProject, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, testOrgID, gotOrg)
	assert.Equal(t, testProjectID, gotProject)
}

func TestResolveAndValidateOrgProject_OrgFromWorkspaceConfig(t *testing.T) {
	srv := buildTestServer(t, listProjectsPath, http.StatusOK, listProjectsSuccessBody)
	extraCfg := fmt.Sprintf(`, "default-workspace": "%s/%s"`, testOrgID, testProjectID)
	cfg := buildTestConfig(t, srv.URL, extraCfg)

	cmd := newTestCmd(t, []string{})

	gotOrg, gotProject, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, testOrgID, gotOrg)
	assert.Equal(t, testProjectID, gotProject)
}

func TestResolveAndValidateOrgProject_MissingOrg(t *testing.T) {
	srv := buildTestServer(t, "", 0, "")
	cfg := buildTestConfig(t, srv.URL, "")

	cmd := newTestCmd(t, []string{"--project-id", testProjectID})

	_, _, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no organization specified")
	assert.Contains(t, err.Error(), "--organization-id")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Equal(t, "Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--organization-id'.", ce.Suggestion)
}

func TestResolveAndValidateOrgProject_MigrationErrorWhenDefaultTenantSet(t *testing.T) {
	srv := buildTestServer(t, "", 0, "")
	extraCfg := `, "default-tenant": "legacy-tenant-id"`
	cfg := buildTestConfig(t, srv.URL, extraCfg)

	cmd := newTestCmd(t, []string{})

	_, _, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default workspace set")
	assert.Contains(t, err.Error(), "aura workspace use")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Equal(t, "Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting.", ce.Suggestion)
}

func TestResolveAndValidateOrgProject_ProjectFromFlag(t *testing.T) {
	srv := buildTestServer(t, listProjectsPath, http.StatusOK, listProjectsSuccessBody)
	cfg := buildTestConfig(t, srv.URL, "")

	cmd := newTestCmd(t, []string{"--organization-id", testOrgID, "--project-id", testProjectID})

	gotOrg, gotProject, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, testOrgID, gotOrg)
	assert.Equal(t, testProjectID, gotProject)
}

func TestResolveAndValidateOrgProject_ProjectFromWorkspaceConfig(t *testing.T) {
	srv := buildTestServer(t, listProjectsPath, http.StatusOK, listProjectsSuccessBody)
	extraCfg := fmt.Sprintf(`, "default-workspace": "%s/%s"`, testOrgID, testProjectID)
	cfg := buildTestConfig(t, srv.URL, extraCfg)

	cmd := newTestCmd(t, []string{})

	gotOrg, gotProject, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.NoError(t, err)
	assert.Equal(t, testOrgID, gotOrg)
	assert.Equal(t, testProjectID, gotProject)
}

func TestResolveAndValidateOrgProject_MissingProject(t *testing.T) {
	srv := buildTestServer(t, "", 0, "")
	cfg := buildTestConfig(t, srv.URL, "")

	cmd := newTestCmd(t, []string{"--organization-id", testOrgID})

	_, _, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no project specified")
	assert.Contains(t, err.Error(), "--project-id")

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Equal(t, "Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--project-id'.", ce.Suggestion)
}

func TestResolveAndValidateOrgProject_ProjectNotInOrg(t *testing.T) {
	srv := buildTestServer(t, listProjectsPath, http.StatusOK, listProjectsEmptyBody)
	cfg := buildTestConfig(t, srv.URL, "")

	cmd := newTestCmd(t, []string{"--organization-id", testOrgID, "--project-id", testProjectID})

	_, _, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not find project")
	assert.Contains(t, err.Error(), testProjectID)
	assert.Contains(t, err.Error(), testOrgID)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Equal(t, "project", ce.ResourceType)
	assert.Equal(t, testProjectID, ce.ResourceID)
	assert.Equal(t, "Run 'neo4j-cli aura project list --organization-id <id>' to see available projects.", ce.Suggestion)
}

func TestResolveAndValidateOrgProject_APIError(t *testing.T) {
	srv := buildTestServer(t, listProjectsPath, http.StatusInternalServerError, `{"errors": [{"message": "internal server error"}]}`)
	cfg := buildTestConfig(t, srv.URL, "")

	cmd := newTestCmd(t, []string{"--organization-id", testOrgID, "--project-id", testProjectID})

	_, _, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal server error")
}

func TestResolveAndValidateOrgProject_TableDrivenResolutionOrder(t *testing.T) {
	for _, tc := range []struct {
		name           string
		args           []string
		extraCfg       string
		serverStatus   int
		serverBody     string
		wantOrg        string
		wantProject    string
		wantErrContain string
	}{
		{
			name:         "flags take precedence over workspace config",
			args:         []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			extraCfg:     `, "default-workspace": "other-org/other-proj"`,
			serverStatus: http.StatusOK,
			serverBody:   listProjectsSuccessBody,
			wantOrg:      testOrgID,
			wantProject:  testProjectID,
		},
		{
			name:           "project-not-in-org returns clear error",
			args:           []string{"--organization-id", testOrgID, "--project-id", "unknown-proj"},
			serverStatus:   http.StatusOK,
			serverBody:     listProjectsSuccessBody,
			wantErrContain: "could not find project unknown-proj in organization " + testOrgID,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildTestServer(t, listProjectsPath, tc.serverStatus, tc.serverBody)
			cfg := buildTestConfig(t, srv.URL, tc.extraCfg)

			cmd := newTestCmd(t, tc.args)

			gotOrg, gotProject, err := utils.ResolveAndValidateOrgProject(cmd, cfg)

			if tc.wantErrContain != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContain)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantOrg, gotOrg)
			assert.Equal(t, tc.wantProject, gotProject)
		})
	}
}

// buildCountingServer creates an httptest.Server that records every request it
// receives, so a caller can assert a resolution path issues no HTTP call at all.
// It answers with a parseable empty list so an unexpected call still reaches the
// request-count assertion instead of panicking inside api.ParseBody.
func buildCountingServer(t *testing.T, requests *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": []}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestResolveOrgID_And_ResolveProjectID pins that each ID resolves on its own,
// so an org-scoped call does not demand a project (and vice versa).
func TestResolveOrgID_And_ResolveProjectID(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args            []string
		extraCfg        string
		wantOrg         string
		wantOrgErr      string
		wantOrgCode     int
		wantProject     string
		wantProjectErr  string
		wantProjectCode int
	}{
		{
			name:            "org flag only",
			args:            []string{"--organization-id", testOrgID},
			wantOrg:         testOrgID,
			wantProjectErr:  "no project specified",
			wantProjectCode: 2,
		},
		{
			name:        "project flag only",
			args:        []string{"--project-id", testProjectID},
			wantOrgErr:  "no organization specified",
			wantOrgCode: 2,
			wantProject: testProjectID,
		},
		{
			name:        "both from default workspace",
			extraCfg:    fmt.Sprintf(`, "default-workspace": "%s/%s"`, testOrgID, testProjectID),
			wantOrg:     testOrgID,
			wantProject: testProjectID,
		},
		{
			name:            "malformed ids",
			args:            []string{"--organization-id", "..", "--project-id", "a/b"},
			wantOrgErr:      `invalid organization id ".."`,
			wantOrgCode:     6,
			wantProjectErr:  `invalid project id "a/b"`,
			wantProjectCode: 6,
		},
		{
			name:            "legacy default-tenant hint is org only",
			extraCfg:        `, "default-tenant": "legacy-tenant-id"`,
			wantOrgErr:      "no default workspace set",
			wantOrgCode:     2,
			wantProjectErr:  "no project specified",
			wantProjectCode: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			srv := buildCountingServer(t, &requests)
			cfg := buildTestConfig(t, srv.URL, tc.extraCfg)
			cmd := newTestCmd(t, tc.args)

			gotOrg, orgErr := utils.ResolveOrgID(cmd, cfg)
			assertResolved(t, tc.wantOrg, tc.wantOrgErr, tc.wantOrgCode, gotOrg, orgErr)

			gotProject, projectErr := utils.ResolveProjectID(cmd, cfg)
			assertResolved(t, tc.wantProject, tc.wantProjectErr, tc.wantProjectCode, gotProject, projectErr)

			assert.Zero(t, requests.Load(), "single-ID resolution must not issue any HTTP request")
		})
	}
}

func assertResolved(t *testing.T, wantID, wantErrContain string, wantCode int, gotID string, err error) {
	t.Helper()

	if wantErrContain != "" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), wantErrContain)
		var ce *clierr.CLIError
		require.True(t, errors.As(err, &ce))
		assert.Equal(t, wantCode, ce.Code)
		return
	}

	require.NoError(t, err)
	assert.Equal(t, wantID, gotID)
}

func TestResolveOrgProject(t *testing.T) {
	for _, tc := range []struct {
		name           string
		args           []string
		extraCfg       string
		wantOrg        string
		wantProject    string
		wantErrContain string
		wantCode       int
		wantSuggestion string
	}{
		{
			name:        "ids from flags",
			args:        []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			wantOrg:     testOrgID,
			wantProject: testProjectID,
		},
		{
			name:        "ids from default workspace",
			extraCfg:    fmt.Sprintf(`, "default-workspace": "%s/%s"`, testOrgID, testProjectID),
			wantOrg:     testOrgID,
			wantProject: testProjectID,
		},
		{
			name:        "flags take precedence over default workspace",
			args:        []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			extraCfg:    `, "default-workspace": "other-org/other-proj"`,
			wantOrg:     testOrgID,
			wantProject: testProjectID,
		},
		{
			name:           "missing organization",
			args:           []string{"--project-id", testProjectID},
			wantErrContain: "no organization specified",
			wantCode:       2,
			wantSuggestion: "Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--organization-id'.",
		},
		{
			name:           "missing project",
			args:           []string{"--organization-id", testOrgID},
			wantErrContain: "no project specified",
			wantCode:       2,
			wantSuggestion: "Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--project-id'.",
		},
		{
			name:           "legacy default-tenant migration hint",
			extraCfg:       `, "default-tenant": "legacy-tenant-id"`,
			wantErrContain: "no default workspace set",
			wantCode:       2,
			wantSuggestion: "Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting.",
		},
		{
			name:           "traversal organization id is rejected",
			args:           []string{"--organization-id", "../..", "--project-id", testProjectID},
			wantErrContain: `invalid organization id "../.."`,
			wantCode:       6,
		},
		{
			name:           "traversal project id is rejected",
			args:           []string{"--organization-id", testOrgID, "--project-id", "a/b"},
			wantErrContain: `invalid project id "a/b"`,
			wantCode:       6,
		},
		{
			name:           "over-segmented default workspace is rejected",
			extraCfg:       `, "default-workspace": "a/b/c"`,
			wantErrContain: `invalid organization id "a/b"`,
			wantCode:       6,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			srv := buildCountingServer(t, &requests)
			cfg := buildTestConfig(t, srv.URL, tc.extraCfg)

			cmd := newTestCmd(t, tc.args)

			gotOrg, gotProject, err := utils.ResolveOrgProject(cmd, cfg)

			if tc.wantErrContain != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContain)
				var ce *clierr.CLIError
				require.True(t, errors.As(err, &ce))
				assert.Equal(t, tc.wantCode, ce.Code)
				assert.Equal(t, tc.wantSuggestion, ce.Suggestion)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantOrg, gotOrg)
				assert.Equal(t, tc.wantProject, gotProject)
			}

			assert.Zero(t, requests.Load(), "ResolveOrgProject must not issue any HTTP request")
		})
	}
}

// buildResourceServer creates an httptest.Server that:
//   - responds to /oauth/token with a dummy token
//   - responds to resourcePath with a 200 body containing the given tenant_id
func buildResourceServer(t *testing.T, resourcePath, tenantID string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"data": {"id": "x", "tenant_id": "%s"}}`, tenantID) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchScopedInstance(t *testing.T) {
	const instanceID = "inst-xyz"
	scopedPath := "/v2beta1/organizations/" + testOrgID + "/projects/" + testProjectID + "/instances/" + instanceID
	srv := buildResourceServer(t, scopedPath, testProjectID)
	cfg := buildTestConfig(t, srv.URL, "")

	body, err := utils.FetchScopedInstance(cfg, testOrgID, testProjectID, instanceID)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"id": "x"`)
}

func TestFetchAndVerifyInstanceInProject_OwnershipMismatch(t *testing.T) {
	const instanceID = "inst-xyz"
	srv := buildResourceServer(t, "/v1/instances/"+instanceID, "other-project")
	cfg := buildTestConfig(t, srv.URL, "")

	_, err := utils.FetchAndVerifyInstanceInProject(cfg, instanceID, testProjectID)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("could not find instance %s in project %s", instanceID, testProjectID), err.Error())

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Equal(t, "instance", ce.ResourceType)
	assert.Equal(t, instanceID, ce.ResourceID)
	assert.Equal(t, "Run 'neo4j-cli aura instance list --project-id <id>' to see instances in this project.", ce.Suggestion)
}

func TestFetchScopedSession(t *testing.T) {
	const sessionID = "sess-xyz"
	scopedPath := "/v2beta1/organizations/" + testOrgID + "/projects/" + testProjectID + "/graph-analytics/sessions/" + sessionID
	srv := buildResourceServer(t, scopedPath, testProjectID)
	cfg := buildTestConfig(t, srv.URL, "")

	body, err := utils.FetchScopedSession(cfg, testOrgID, testProjectID, sessionID)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"id": "x"`)
}

func TestFetchAndVerifyCMKInProject_OwnershipMismatch(t *testing.T) {
	const cmkID = "cmk-xyz"
	srv := buildResourceServer(t, "/v1/customer-managed-keys/"+cmkID, "other-project")
	cfg := buildTestConfig(t, srv.URL, "")

	_, err := utils.FetchAndVerifyCMKInProject(cfg, cmkID, testProjectID)
	require.Error(t, err)
	assert.Equal(t, fmt.Sprintf("could not find customer-managed-key %s in project %s", cmkID, testProjectID), err.Error())

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 3, ce.Code)
	assert.Equal(t, "customer-managed-key", ce.ResourceType)
	assert.Equal(t, cmkID, ce.ResourceID)
	assert.Equal(t, "Run 'neo4j-cli aura customer-managed-key list --project-id <id>' to see keys in this project.", ce.Suggestion)
}

func TestValidateResourceID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "valid short id", id: "2f49c2b3", wantErr: false},
		{name: "valid uuid", id: "11111111-1111-1111-1111-111111111111", wantErr: false},
		{name: "dots within a segment are fine", id: "a..b", wantErr: false},
		{name: "empty", id: "", wantErr: true},
		{name: "dot segment", id: ".", wantErr: true},
		{name: "dotdot segment", id: "..", wantErr: true},
		{name: "traversal", id: "../../../target", wantErr: true},
		{name: "embedded slash", id: "a/b", wantErr: true},
		{name: "embedded backslash", id: `a\b`, wantErr: true},
		{name: "query separator", id: "x?admin=true", wantErr: true},
		{name: "fragment separator", id: "x#frag", wantErr: true},
		{name: "percent escape", id: "x%2e%2e", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := utils.ValidateResourceID("instance", tc.id)
			if tc.wantErr {
				require.Error(t, err)
				var ce *clierr.CLIError
				require.True(t, errors.As(err, &ce))
				assert.Contains(t, ce.Message, fmt.Sprintf("invalid instance id %q", tc.id))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
