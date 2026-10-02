// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

import (
	"github.com/neo4j/cli/internal/auraclient"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of customer managed keys",
		Long: `This subcommand returns a list containing a summary of each of your customer managed keys in the specified project. To find out more about a specific key, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's keys to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List all customer managed keys in a project
neo4j-cli aura customer-managed-key list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List keys using a configured default workspace
neo4j-cli aura customer-managed-key list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura customer-managed-key list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			keys, err := auraclient.New(cfg).CustomerManagedKeys().List(cmd.Context(), auraclient.Scope{ProjectID: projectID})
			if err != nil {
				return err
			}

			rows := make([]map[string]any, len(keys))
			for i, k := range keys {
				rows[i] = k.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "name", "project_id"})

			return nil
		},
	}

	return cmd
}
