// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment

import (
	"strings"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Delete the given deployment",
		Long: `Deletes the given Fleet Manager deployment. This only removes the deployment from Fleet Manager; the running database is not affected. Disable Fleet Management in the database afterwards with ` + "`call fleetManagement.disable()`" + `.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete a deployment in the default project
neo4j-cli aura deployment delete 22222222-2222-2222-2222-222222222222 --rw --yes --force

# Delete a deployment in a specific project
neo4j-cli aura deployment delete 22222222-2222-2222-2222-222222222222 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			id := strings.TrimSpace(args[0])

			if err := auraclient.ValidateResourceID("deployment", id); err != nil {
				return err
			}
			if err := confirm.Require(cmd, id); err != nil {
				return err
			}
			if err := auraclient.New(cfg).Deployments().Delete(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, id); err != nil {
				return err
			}
			cmd.Println("Deployment deleted successfully", id)
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
