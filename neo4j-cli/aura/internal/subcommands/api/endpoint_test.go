// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	auraflags "github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOrgID     = "org-abc-123"
	testProjectID = "proj-def-456"
)

// newEndpointTestConfig builds a config with no base-url and no credentials:
// endpoint resolution must never issue a request, so any regression that does
// fails loudly rather than reaching a live host.
func newEndpointTestConfig(t *testing.T, extraAuraCfg string) *clicfg.Config {
	t.Helper()

	cfgJSON := fmt.Sprintf(`{"format": "json", "aura": {"auth-url": "", "base-url": ""%s}}`, extraAuraCfg)
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)

	return clicfg.NewConfig(fs, "test", clicfg.AuraScope)
}

// newEndpointTestCmd registers the org/project flags and parses args so
// Flags().GetString sees them, mirroring how the api command is mounted.
func newEndpointTestCmd(t *testing.T, args []string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{
		Use:  "api",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	auraflags.RegisterOrgProjectFlags(cmd)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())

	return cmd
}

func TestParseEndpoint(t *testing.T) {
	for _, tt := range []struct {
		name        string
		endpoint    string
		versionPath string
		path        string
		query       url.Values
	}{
		{
			name:        "version and path",
			endpoint:    "v1/instances",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{},
		},
		{
			name:        "leading slash accepted",
			endpoint:    "/v1/instances",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{},
		},
		{
			name:        "surrounding whitespace trimmed",
			endpoint:    "  v1/instances  ",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{},
		},
		{
			name:        "arbitrary version segment is literal",
			endpoint:    "v9alpha3/organizations",
			versionPath: "v9alpha3",
			path:        "organizations",
			query:       url.Values{},
		},
		{
			name:        "version only",
			endpoint:    "v1",
			versionPath: "v1",
			path:        "",
			query:       url.Values{},
		},
		{
			name:        "deep path",
			endpoint:    "v2beta1/organizations/o/projects/p/instances/i",
			versionPath: "v2beta1",
			path:        "organizations/o/projects/p/instances/i",
			query:       url.Values{},
		},
		{
			name:        "trailing slash preserved",
			endpoint:    "v1/instances/",
			versionPath: "v1",
			path:        "instances/",
			query:       url.Values{},
		},
		{
			name:        "inline query split off",
			endpoint:    "v1/instances?include_deleted=true",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{"include_deleted": []string{"true"}},
		},
		{
			name:        "repeated query key preserved",
			endpoint:    "v1/instances?id=a&id=b&page_limit=10",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{"id": []string{"a", "b"}, "page_limit": []string{"10"}},
		},
		{
			name:        "empty query string",
			endpoint:    "v1/instances?",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{},
		},
		{
			name:        "valueless query key is normalized",
			endpoint:    "v1/instances?include_deleted",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{"include_deleted": []string{""}},
		},
		{
			name:        "plus in a query value decodes to a space",
			endpoint:    "v1/instances?name=my+db",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{"name": []string{"my db"}},
		},
		{
			name:        "percent encoded segment kept escaped",
			endpoint:    "v1/instances/a%2Fb",
			versionPath: "v1",
			path:        "instances/a%2Fb",
			query:       url.Values{},
		},
		{
			name:        "dot inside a segment is allowed",
			endpoint:    "v1/instances/my.instance",
			versionPath: "v1",
			path:        "instances/my.instance",
			query:       url.Values{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEndpoint(tt.endpoint)
			require.NoError(t, err)
			assert.Equal(t, tt.versionPath, got.versionPath)
			assert.Equal(t, tt.path, got.path)
			assert.Equal(t, tt.query, got.query)
		})
	}
}

func TestParseEndpoint_UsageErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint string
		contains string
	}{
		{name: "empty", endpoint: "", contains: "endpoint is required"},
		{name: "whitespace only", endpoint: "   ", contains: "endpoint is required"},
		{name: "slash only", endpoint: "/", contains: "has no path"},
		{name: "query only", endpoint: "?include_deleted=true", contains: "has no path"},
		{name: "https url", endpoint: "https://evil.example.com/v1/instances", contains: "not an absolute URL"},
		{name: "http url", endpoint: "http://evil.example.com/v1/instances", contains: "not an absolute URL"},
		{name: "scheme relative", endpoint: "//evil.example.com/v1/instances", contains: "not an absolute URL"},
		{name: "scheme only", endpoint: "file://x", contains: "not an absolute URL"},
		{name: "host with port as scheme", endpoint: "evil.example.com:8080/v1/instances", contains: "not an absolute URL"},
		{name: "parent segment", endpoint: "v1/../secret", contains: `must not contain a ".." path segment`},
		{name: "leading parent segment", endpoint: "../v1/instances", contains: `must not contain a ".." path segment`},
		{name: "current segment", endpoint: "v1/./instances", contains: `must not contain a "." path segment`},
		{name: "encoded parent segment", endpoint: "v1/%2e%2e/secret", contains: `must not contain a ".." path segment`},
		{name: "fragment", endpoint: "v1/instances#section", contains: "must not contain a '#' fragment"},
		{name: "invalid path escape", endpoint: "v1/instances%zz", contains: "is not a valid path"},
		{name: "invalid query escape", endpoint: "v1/instances?a=%zz", contains: "query string"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEndpoint(tt.endpoint)
			assert.Nil(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 2, ce.Code)
		})
	}
}

func TestResolveEndpoint_Substitution(t *testing.T) {
	workspace := fmt.Sprintf(`, "default-workspace": "%s/%s"`, testOrgID, testProjectID)

	for _, tt := range []struct {
		name        string
		endpoint    string
		args        []string
		extraCfg    string
		versionPath string
		path        string
		query       url.Values
	}{
		{
			name:        "snake case tokens from flags",
			endpoint:    "v2beta1/organizations/{org_id}/projects/{project_id}/instances",
			args:        []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			versionPath: "v2beta1",
			path:        "organizations/" + testOrgID + "/projects/" + testProjectID + "/instances",
			query:       url.Values{},
		},
		{
			name:        "short alias tokens from flags",
			endpoint:    "v2beta1/organizations/{org}/projects/{project}/instances",
			args:        []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			versionPath: "v2beta1",
			path:        "organizations/" + testOrgID + "/projects/" + testProjectID + "/instances",
			query:       url.Values{},
		},
		{
			name:        "tokens from default workspace",
			endpoint:    "/v2beta1/organizations/{org_id}/projects/{project_id}/instances",
			extraCfg:    workspace,
			versionPath: "v2beta1",
			path:        "organizations/" + testOrgID + "/projects/" + testProjectID + "/instances",
			query:       url.Values{},
		},
		{
			name:        "flags win over default workspace",
			endpoint:    "v2beta1/organizations/{org_id}/projects/{project_id}",
			args:        []string{"--organization-id", "org-flag", "--project-id", "proj-flag"},
			extraCfg:    workspace,
			versionPath: "v2beta1",
			path:        "organizations/org-flag/projects/proj-flag",
			query:       url.Values{},
		},
		{
			name:        "repeated token substituted everywhere",
			endpoint:    "v2beta1/organizations/{org_id}/x/{org}",
			args:        []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			versionPath: "v2beta1",
			path:        "organizations/" + testOrgID + "/x/" + testOrgID,
			query:       url.Values{},
		},
		{
			name:        "token inside query value",
			endpoint:    "v1/instances?tenantId={project_id}",
			args:        []string{"--organization-id", testOrgID, "--project-id", testProjectID},
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{"tenantId": []string{testProjectID}},
		},
		{
			name:        "org placeholder alone needs no project",
			endpoint:    "v2beta1/organizations/{org_id}/projects",
			args:        []string{"--organization-id", testOrgID},
			versionPath: "v2beta1",
			path:        "organizations/" + testOrgID + "/projects",
			query:       url.Values{},
		},
		{
			name:        "project placeholder alone needs no organization",
			endpoint:    "v1/instances?tenantId={project_id}",
			args:        []string{"--project-id", testProjectID},
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{"tenantId": []string{testProjectID}},
		},
		{
			name:        "no placeholder needs no workspace",
			endpoint:    "v1/instances",
			versionPath: "v1",
			path:        "instances",
			query:       url.Values{},
		},
		{
			name:        "literal ids need no workspace",
			endpoint:    "v2beta1/organizations/o/projects/p/instances",
			versionPath: "v2beta1",
			path:        "organizations/o/projects/p/instances",
			query:       url.Values{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newEndpointTestConfig(t, tt.extraCfg)
			cmd := newEndpointTestCmd(t, tt.args)

			got, err := resolveEndpoint(cmd, cfg, tt.endpoint)
			require.NoError(t, err)
			assert.Equal(t, tt.versionPath, got.versionPath)
			assert.Equal(t, tt.path, got.path)
			assert.Equal(t, tt.query, got.query)
		})
	}
}

func TestResolveEndpoint_ResolutionErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint string
		args     []string
		extraCfg string
		contains string
		exitCode int
	}{
		{
			name:     "missing organization",
			endpoint: "v2beta1/organizations/{org_id}/projects",
			contains: "no organization specified",
			exitCode: 2,
		},
		{
			name:     "missing project",
			endpoint: "v2beta1/organizations/{org_id}/projects/{project_id}",
			args:     []string{"--organization-id", testOrgID},
			contains: "no project specified",
			exitCode: 2,
		},
		{
			name:     "legacy default tenant hint",
			endpoint: "v2beta1/organizations/{org}/projects",
			extraCfg: `, "default-tenant": "legacy-tenant-id"`,
			contains: "no default workspace set",
			exitCode: 2,
		},
		{
			name:     "traversal organization id",
			endpoint: "v2beta1/organizations/{org_id}/projects/{project_id}",
			args:     []string{"--organization-id", "..", "--project-id", testProjectID},
			contains: `invalid organization id ".."`,
			exitCode: 6,
		},
		{
			name:     "traversal project id",
			endpoint: "v2beta1/organizations/{org_id}/projects/{project_id}",
			args:     []string{"--organization-id", testOrgID, "--project-id", "a/b"},
			contains: `invalid project id "a/b"`,
			exitCode: 6,
		},
		{
			name:     "query injecting organization id",
			endpoint: "v2beta1/organizations/{org_id}/projects",
			args:     []string{"--organization-id", "x?admin=true"},
			contains: `invalid organization id "x?admin=true"`,
			exitCode: 6,
		},
		{
			name:     "escape injecting project id",
			endpoint: "v1/instances/{project_id}",
			args:     []string{"--project-id", "x%2e%2e"},
			contains: `invalid project id "x%2e%2e"`,
			exitCode: 6,
		},
		{
			name:     "unsupported placeholder",
			endpoint: "v2beta1/organizations/{organization_id}/projects",
			contains: "unsupported placeholder {organization_id}",
			exitCode: 2,
		},
		{
			name:     "unsupported placeholder alongside a supported one",
			endpoint: "v2beta1/organizations/{org_id}/projects/{proj}",
			args:     []string{"--organization-id", testOrgID},
			contains: "unsupported placeholder {proj}",
			exitCode: 2,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newEndpointTestConfig(t, tt.extraCfg)
			cmd := newEndpointTestCmd(t, tt.args)

			got, err := resolveEndpoint(cmd, cfg, tt.endpoint)
			assert.Nil(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)

			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, tt.exitCode, ce.Code)
		})
	}
}
