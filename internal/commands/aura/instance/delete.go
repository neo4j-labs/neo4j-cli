// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "delete <id>",
		Short:       "Deletes an instance",
		Annotations: map[string]string{"write": "true"},
		Long: `Starts the deletion process of an Aura instance.

Deleting an instance is an asynchronous operation. You can poll the current status of this operation by periodically getting the instance details for the instance ID using the get subcommand.

If another operation is being performed on the instance you are trying to delete, an error will be returned that indicates that deletion cannot be performed.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.

--dry-run looks the instance up and prints what would be deleted without sending the DELETE. It needs no --yes --force.`,
		Example: `# Delete an instance by ID
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Preview the deletion without deleting anything
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --dry-run --format json

# Delete an instance and emit the response as JSON
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json

# Delete and pipe the response status through jq
neo4j-cli aura instance delete 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json | jq -r '.data.status'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			instanceID := strings.TrimSpace(args[0])

			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			if err := auraclient.ValidateResourceID("instance", instanceID); err != nil {
				return err
			}

			if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
				inst, err := auraclient.New(cfg).Instances().Get(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, instanceID)
				if err != nil {
					return err
				}
				output.PrintRecord(cmd, cfg, map[string]any{
					"dry_run":       true,
					"action":        "delete",
					"resource_type": "instance",
					"resource_id":   inst.ID,
					"name":          inst.Name,
				}, []string{"dry_run", "action", "resource_type", "resource_id", "name"})
				return nil
			}

			if err := confirm.Require(cmd, instanceID); err != nil {
				return err
			}

			inst, err := auraclient.New(cfg).Instances().Delete(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, instanceID)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, inst.Record, []string{"id", "name", "project_id", "status", "connection_url", "cloud_provider", "region", "type", "memory"})

			return nil
		},
	}

	confirm.Register(cmd)
	cmd.Flags().Bool("dry-run", false, "Look up the instance and print what would be deleted without deleting it")

	return cmd
}
