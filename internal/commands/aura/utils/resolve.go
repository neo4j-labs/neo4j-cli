// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"
	"net/http"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/flags"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
)

// ResolveAndValidateOrgProject resolves the organization and project IDs for
// Aura commands via ResolveOrgProject (see there for the precedence), then calls
// GET /organizations/{orgID}/projects (v2beta1) and returns an error when the
// resolved projectID is not found in the list.
func ResolveAndValidateOrgProject(cmd *cobra.Command, cfg *clicfg.Config) (orgID, projectID string, err error) {
	orgID, projectID, err = ResolveOrgProject(cmd, cfg)
	if err != nil {
		return "", "", err
	}

	if err = validateProjectInOrg(cfg, orgID, projectID); err != nil {
		return "", "", err
	}

	return orgID, projectID, nil
}

// ResolveOrgProject resolves the organization and project IDs via ResolveOrgID
// and ResolveProjectID (see those for the precedence), reporting a missing
// organization before a missing project.
//
// Both IDs are validated, but no HTTP request is issued: project membership is
// left to the scoped request the caller is about to make. Use it when an extra
// GET /organizations/{orgID}/projects round trip would buy nothing.
func ResolveOrgProject(cmd *cobra.Command, cfg *clicfg.Config) (orgID, projectID string, err error) {
	orgID, err = ResolveOrgID(cmd, cfg)
	if err != nil {
		return "", "", err
	}

	projectID, err = ResolveProjectID(cmd, cfg)
	if err != nil {
		return "", "", err
	}

	return orgID, projectID, nil
}

// ResolveOrgID resolves and validates the organization ID from (1) the
// --organization-id flag; (2) the org portion of aura.default-workspace;
// otherwise a usage error — replaced by a migration hint when the legacy
// aura.default-tenant is set and aura.default-workspace is not. No HTTP request
// is issued. Use it over ResolveOrgProject when the request needs no project, so
// an org-scoped call (e.g. listing the projects in an org) does not demand one.
func ResolveOrgID(cmd *cobra.Command, cfg *clicfg.Config) (string, error) {
	defaultOrg, _ := defaultOrgAndProject(cfg)

	var orgID string
	if flagVal, _ := cmd.Flags().GetString(flags.OrgIDFlag); flagVal != "" {
		orgID = flagVal
	} else if defaultOrg != "" {
		orgID = defaultOrg
	} else {
		// Check for legacy default-tenant before returning generic error.
		if cfg.Aura.Get("default-tenant") != nil && cfg.Aura.Get("default-tenant") != "" {
			return "", clierr.NewUsageError("no default workspace set; run 'aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting").
				WithSuggestion("Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting.")
		}
		return "", clierr.NewUsageError("no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'").
			WithSuggestion("Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--organization-id'.")
	}

	// Reject malformed IDs before they reach a request path, so a "." / ".." /
	// slash segment can't retarget it (see ValidateResourceID).
	if err := aura.ValidateResourceID("organization", orgID); err != nil {
		return "", err
	}

	return orgID, nil
}

// ResolveProjectID resolves and validates the project ID from (1) the
// --project-id flag; (2) the project portion of aura.default-workspace;
// otherwise a usage error. No HTTP request is issued.
func ResolveProjectID(cmd *cobra.Command, cfg *clicfg.Config) (string, error) {
	_, defaultProject := defaultOrgAndProject(cfg)

	var projectID string
	if flagVal, _ := cmd.Flags().GetString(flags.ProjectIDFlag); flagVal != "" {
		projectID = flagVal
	} else if defaultProject != "" {
		projectID = defaultProject
	} else {
		return "", clierr.NewUsageError("no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'").
			WithSuggestion("Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--project-id'.")
	}

	if err := aura.ValidateResourceID("project", projectID); err != nil {
		return "", err
	}

	return projectID, nil
}

// FetchProjectInOrg derives a single project from the v2beta1 list-projects
// endpoint (GET /organizations/{orgID}/projects) by filtering for projectID.
// It is the canonical "resolve one project" path shared by project get and the
// org/project scope validation: an absent project yields a structured
// clierr.NewNotFoundError with resource + suggestion.
func FetchProjectInOrg(cfg *clicfg.Config, orgID, projectID string) (*api.Project, error) {
	projects, err := api.ListProjects(cfg, orgID)
	if err != nil {
		return nil, err
	}

	for i := range projects.Data {
		if projects.Data[i].Id == projectID {
			return &projects.Data[i], nil
		}
	}

	return nil, clierr.NewNotFoundError("could not find project %s in organization %s", projectID, orgID).
		WithResource("project", projectID).
		WithSuggestion("Run 'neo4j-cli aura project list --organization-id <id>' to see available projects.")
}

// validateProjectInOrg confirms that projectID appears in the org's project
// list, reusing the canonical FetchProjectInOrg lookup.
func validateProjectInOrg(cfg *clicfg.Config, orgID, projectID string) error {
	_, err := FetchProjectInOrg(cfg, orgID, projectID)
	return err
}

// OrgFromWorkspace returns the organization portion of aura.default-workspace,
// or an empty string when the workspace is not set or has no '/'. It is the
// canonical org-parsing helper shared by project commands.
func OrgFromWorkspace(cfg *clicfg.Config) string {
	orgID, _ := defaultOrgAndProject(cfg)
	return orgID
}

// FetchAndVerifyCMKInProject performs a GET /customer-managed-keys/{cmkID} and
// checks that the key's tenant_id matches projectID. It returns the raw
// response body so the caller can reuse it for output (avoiding a second
// round-trip in read-only commands such as "customer-managed-key get").
//
// If the key exists but belongs to a different project the function
// returns (nil, "could not find customer-managed-key {cmkID} in project {projectID}").
func FetchAndVerifyCMKInProject(cfg *clicfg.Config, cmkID, projectID string) ([]byte, error) {
	path := fmt.Sprintf("/customer-managed-keys/%s", cmkID)
	resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
		Method: http.MethodGet,
	})
	if err != nil {
		return nil, err
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from preflight ownership check", statusCode)
	}

	responseData := api.ParseBody(resBody)
	cmk, err := responseData.GetSingleOrError()
	if err != nil {
		return nil, err
	}

	tenantID, _ := cmk["tenant_id"].(string)
	if tenantID != projectID {
		return nil, clierr.NewNotFoundError("could not find customer-managed-key %s in project %s", cmkID, projectID).
			WithResource("customer-managed-key", cmkID).
			WithSuggestion("Run 'neo4j-cli aura customer-managed-key list --project-id <id>' to see keys in this project.")
	}

	return resBody, nil
}

// ResolveAndVerifyInstance resolves the organization and project (flags, then
// the default workspace), validates them, and checks that the instance belongs
// to the project. It is the single preflight for leaves that operate on
// instance-scoped endpoints which are not themselves project-scoped (the
// GraphQL Data API family, snapshots, ...).
func ResolveAndVerifyInstance(cmd *cobra.Command, cfg *clicfg.Config, instanceID string) (aura.Scope, error) {
	orgID, projectID, err := ResolveAndValidateOrgProject(cmd, cfg)
	if err != nil {
		return aura.Scope{}, err
	}
	scope := aura.Scope{OrgID: orgID, ProjectID: projectID}
	if err := aura.New(cfg).Instances().Verify(cmd.Context(), scope, instanceID); err != nil {
		return aura.Scope{}, err
	}
	return scope, nil
}
