// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import (
	"context"
	"github.com/neo4j/cli/internal/aura"
	"strings"

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

// validateProjectInOrg confirms that projectID appears in the org's project
// list, reusing the canonical FetchProjectInOrg lookup.
func validateProjectInOrg(cfg *clicfg.Config, orgID, projectID string) error {
	_, err := aura.New(cfg).Projects().Get(context.Background(), orgID, projectID)
	return err
}

// defaultOrgAndProject parses the aura.default-workspace slug ("{orgId}/{projectId}")
// and returns the org and project portions. Returns empty strings when the workspace
// is not set or does not contain a '/'.
func defaultOrgAndProject(cfg *clicfg.Config) (orgID, projectID string) {
	ctx := cfg.Aura.DefaultWorkspace()
	if ctx == "" {
		return "", ""
	}
	idx := strings.LastIndex(ctx, "/")
	if idx < 0 {
		return "", ""
	}
	return ctx[:idx], ctx[idx+1:]
}

// OrgFromWorkspace returns the organization portion of aura.default-workspace,
// or an empty string when the workspace is not set or has no '/'. It is the
// canonical org-parsing helper shared by project commands.
func OrgFromWorkspace(cfg *clicfg.Config) string {
	orgID, _ := defaultOrgAndProject(cfg)
	return orgID
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
