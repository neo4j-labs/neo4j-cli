// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
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

			found, err := aura.New(cfg).Projects().Get(cmd.Context(), orgID, projectID)
			if err != nil {
				return err
			}

			// Only id and name are modelled for a project; output carries exactly those.
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(map[string]any{"id": found.ID, "name": found.Name}), []string{"id", "name"})

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID (defaults to org portion of aura.default-workspace)")

	return cmd
}
