// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/aura"
	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of instances",
		Long: `This subcommand returns a list containing a summary of each of your Aura instances in the specified project. To find out more about a specific instance, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's instances to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List instances in a project (using flags)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List instances using a configured default workspace
neo4j-cli aura instance list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			instances, err := aura.New(cfg).Instances().List(cmd.Context(), aura.Scope{OrgID: orgID, ProjectID: projectID})
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(instances))
			for i, inst := range instances {
				rows[i] = inst.Record
			}
			output.PrintBodyMap(cmd, cfg, api.NewListResponseData(rows), []string{"id", "name", "status", "organization_id", "project_id", "cloud_provider"})
			return nil
		},
	}

	return cmd
}
