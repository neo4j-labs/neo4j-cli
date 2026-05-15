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
		Example: `# Get project details (using default organization from aura.default-context)
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000

# Get project details with an explicit organization ID
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000001

# Emit JSON for scripting
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000001 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]

			orgID := organizationId
			if orgID == "" {
				orgID = resolveOrgFromContext(cfg)
			}
			if orgID == "" {
				return fmt.Errorf("required flag \"organization-id\" not set and aura.default-context is not configured")
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, fmt.Sprintf("/organizations/%s/projects/%s", orgID, projectID), &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion2,
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

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID (defaults to org portion of aura.default-context)")

	return cmd
}
