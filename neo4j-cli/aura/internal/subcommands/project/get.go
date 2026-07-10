// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
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
			projectID := strings.TrimSpace(args[0])

			orgID := organizationId
			if orgID == "" {
				orgID = utils.OrgFromWorkspace(cfg)
			}
			if orgID == "" {
				return fmt.Errorf("required flag \"organization-id\" not set and aura.default-workspace is not configured")
			}

			cmd.SilenceUsage = true

			found, err := utils.FetchProjectInOrg(cfg, orgID, projectID)
			if err != nil {
				return err
			}

			resBody, err := json.Marshal(api.GetProjectResponse{Data: *found})
			if err != nil {
				return err
			}

			output.PrintBody(cmd, cfg, resBody, []string{"id", "name"})

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID (defaults to org portion of aura.default-workspace)")

	return cmd
}
