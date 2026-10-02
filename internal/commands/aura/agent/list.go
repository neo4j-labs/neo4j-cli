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

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of agents",
		Long:  "Returns a list of agents for the specified project.",
		Example: `# List all agents in the default project
neo4j-cli aura agent list

# List agents in a specific organization and project
neo4j-cli aura agent list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# List agents as JSON for scripting
neo4j-cli aura agent list --format json`,
		Args: cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			agents, err := aura.New(cfg).Agents().List(cmd.Context(), aura.Scope{OrgID: organizationId, ProjectID: projectId})
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(agents))
			for i, a := range agents {
				rows[i] = a.Record
			}
			output.PrintBodyMap(cmd, cfg, api.NewListResponseData(rows), []string{"id", "name", "description", "dbid", "enabled"})

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "Project ID")

	return cmd
}
