// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/spf13/cobra"
)

// ResolveAndValidateOrgProject resolves the organization and project IDs for
// Aura commands using the following precedence:
//
//   - Organization ID: (1) --organization-id flag; (2) org portion of
//     aura.default-workspace; (3) error.
//   - Project ID: (1) --project-id flag; (2) deprecated --tenant-id flag;
//     (3) project portion of aura.default-workspace; (4) if
//     aura.default-tenant is set but aura.default-workspace is not, return a
//     migration message; (5) error.
//
// After resolving both IDs it calls GET /organizations/{orgID}/projects
// (v2beta1) and returns an error when the resolved projectID is not found in
// the list.
func ResolveAndValidateOrgProject(cmd *cobra.Command, cfg *clicfg.Config) (orgID, projectID string, err error) {
	orgID, projectID, err = resolveIDs(cmd, cfg)
	if err != nil {
		return "", "", err
	}

	if err = validateProjectInOrg(cfg, orgID, projectID); err != nil {
		return "", "", err
	}

	return orgID, projectID, nil
}

// resolveIDs applies the flag + config resolution order and returns (orgID,
// projectID). It does NOT make any API calls.
func resolveIDs(cmd *cobra.Command, cfg *clicfg.Config) (orgID, projectID string, err error) {
	defaultOrg, defaultProject := defaultOrgAndProject(cfg)

	// Resolve org ID.
	if flagVal, _ := cmd.Flags().GetString(flags.OrgIDFlag); flagVal != "" {
		orgID = flagVal
	} else if defaultOrg != "" {
		orgID = defaultOrg
	} else {
		// Check for legacy default-tenant before returning generic error.
		if cfg.Aura.Get("default-tenant") != nil && cfg.Aura.Get("default-tenant") != "" {
			return "", "", fmt.Errorf("no default workspace set; run 'aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting")
		}
		return "", "", fmt.Errorf("no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
	}

	// Resolve project ID.
	if flagVal, _ := cmd.Flags().GetString(flags.ProjectIDFlag); flagVal != "" {
		projectID = flagVal
	} else if tenantVal, _ := cmd.Flags().GetString(flags.TenantIDFlag); tenantVal != "" {
		projectID = tenantVal
	} else if defaultProject != "" {
		projectID = defaultProject
	} else {
		return "", "", fmt.Errorf("no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
	}

	return orgID, projectID, nil
}

// validateProjectInOrg calls the v2beta1 list-projects endpoint and confirms
// that projectID appears in the response.
func validateProjectInOrg(cfg *clicfg.Config, orgID, projectID string) error {
	projects, err := api.ListProjects(cfg, orgID)
	if err != nil {
		return err
	}

	for _, p := range projects.Data {
		if p.Id == projectID {
			return nil
		}
	}

	return fmt.Errorf("could not find project %s in organization %s", projectID, orgID)
}

// FetchAndVerifyInstanceInProject performs a GET /instances/{instanceID} and
// checks that the instance's tenant_id matches projectID. It returns the raw
// response body so the caller can reuse it for output (avoiding a second
// round-trip in read-only commands such as "instance get").
//
// If the instance exists but belongs to a different project the function
// returns (nil, "could not find instance {instanceID} in project {projectID}").
func FetchAndVerifyInstanceInProject(cfg *clicfg.Config, instanceID, projectID string) ([]byte, error) {
	path := fmt.Sprintf("/instances/%s", instanceID)
	resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
		Method: http.MethodGet,
	})
	if err != nil {
		return nil, err
	}

	if statusCode != http.StatusOK {
		// Non-200 was already turned into an error by MakeRequest; reaching
		// here with a non-200 should not happen, but guard defensively.
		return resBody, nil
	}

	responseData := api.ParseBody(resBody)
	instance, err := responseData.GetSingleOrError()
	if err != nil {
		return nil, err
	}

	tenantID, _ := instance["tenant_id"].(string)
	if tenantID != projectID {
		return nil, fmt.Errorf("could not find instance %s in project %s", instanceID, projectID)
	}

	return resBody, nil
}
