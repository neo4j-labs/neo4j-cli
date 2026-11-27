// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns all deployments",
		Long:  "Returns all Fleet Manager deployments for the given project.",
		Example: `# List deployments in the default project
neo4j-cli aura deployment list

# List deployments in a specific project
neo4j-cli aura deployment list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List deployments as JSON for scripting
neo4j-cli aura deployment list --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			items, err := auraclient.New(cfg).Deployments().List(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID})
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(items))
			for i, d := range items {
				rows[i] = d.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "name", "created_by", "status", "connection_url"})
			return nil
		},
	}
}
