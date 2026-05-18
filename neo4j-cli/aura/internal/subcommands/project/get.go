// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	var organizationId string

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Returns project details",
		Long:  "This subcommand returns details about a specific Aura project.",
		Example: `# Get project details by ID (uses org from aura.default-workspace)
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000

# Get project details in a specific organization
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --organization-id 11111111-1111-1111-1111-111111111111

# Emit JSON for scripting
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]

			orgID := organizationId
			if orgID == "" {
				orgID = resolveOrgFromWorkspace(cfg)
			}
			if orgID == "" {
				return fmt.Errorf("required flag \"organization-id\" not set and aura.default-workspace is not configured")
			}

			cmd.SilenceUsage = true

			projects, err := api.ListProjects(cfg, orgID)
			if err != nil {
				return err
			}
			found := false
			for _, p := range projects.Data {
				if p.Id == projectID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("project %s not found in organization %s", projectID, orgID)
			}

			resBody, statusCode, err := api.MakeRequest(cfg, fmt.Sprintf("/tenants/%s", projectID), &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion1,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name"})
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID (defaults to org portion of aura.default-workspace)")

	return cmd
}
