// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var organizationId string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of projects",
		Long:  "This subcommand returns a list of Aura projects within the given organization.",
		Example: `# List all projects in the default organization (from aura.default-workspace)
neo4j-cli aura project list

# List projects in a specific organization
neo4j-cli aura project list --organization-id 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting
neo4j-cli aura project list --organization-id 00000000-0000-0000-0000-000000000000 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			orgID := organizationId
			if orgID == "" {
				orgID = utils.OrgFromWorkspace(cfg)
			}
			if orgID == "" {
				return fmt.Errorf("required flag \"organization-id\" not set and aura.default-workspace is not configured")
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, fmt.Sprintf("/organizations/%s/projects", orgID), &api.RequestConfig{
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

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID (defaults to org portion of aura.default-workspace)")

	return cmd
}
