// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/internal/aura"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Returns agent details",
		Long:  "Returns the details of a specific agent.",
		Example: `# Get details for an agent
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000

# Get an agent in a specific organization and project
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# Get agent details as JSON for scripting
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			agent, err := aura.New(cfg).Agents().Get(cmd.Context(), aura.Scope{OrgID: organizationId, ProjectID: projectId}, args[0])
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(agent.Record), []string{"id", "name", "description", "dbid", "is_private", "is_mcp_enabled", "enabled"})

			return nil
		},
	}

	return cmd
}
