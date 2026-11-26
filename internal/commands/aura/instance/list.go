// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of instances",
		Long: `This subcommand returns a list containing a summary of each of your Aura instances in the specified project. To find out more about a specific instance, retrieve the details using the get subcommand.

Use --limit to return at most that many instances, in which case a note is written to stderr saying more instances exist.

Use --organization-id and --project-id to specify which project's instances to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List instances in a project (using flags)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List instances using a configured default workspace
neo4j-cli aura instance list

# Return at most 10 instances and emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --limit 10 --format json

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			page, err := auraclient.New(cfg).Instances().List(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, limit)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(page.Items))
			for i, inst := range page.Items {
				rows[i] = inst.Record
			}
			output.PrintPagedRecords(cmd, cfg, rows, []string{"id", "name", "status", "organization_id", "project_id", "cloud_provider"}, "instances", limit, page.LimitReached, false)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "The maximum number of instances to return. Omit to return all of them.")

	return cmd
}
