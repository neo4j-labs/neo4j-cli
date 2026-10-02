// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"github.com/neo4j/cli/internal/aura"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of Graph Analytics Serverless sessions",
		Example: `# List all Graph Analytics sessions in a project
neo4j-cli aura graph-analytics session list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List sessions using a configured default workspace
neo4j-cli aura graph-analytics session list

# List sessions attached to a specific instance and emit JSON for scripting
neo4j-cli aura graph-analytics session list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --instance-id 00000000 --format json`,
		Long: `This subcommand returns a list containing a summary of each of your Graph Analytics Serverless sessions in the specified project.

Use --organization-id and --project-id to specify which project's sessions to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			sessions, err := aura.New(cfg).Sessions().List(cmd.Context(), aura.Scope{OrgID: orgID, ProjectID: projectID}, instanceId)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(sessions))
			for i, sess := range sessions {
				rows[i] = sess.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "name", "status", "project_id", "cloud_provider", "ttl"})
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "An optional Instance ID to filter for sessions attached to an instance")

	return cmd
}
