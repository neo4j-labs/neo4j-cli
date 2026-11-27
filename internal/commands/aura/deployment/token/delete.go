// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/deployment/internal/depflag"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete",
		Short:       "Delete the deployment token",
		Long: `Deletes the token of the given Fleet Manager deployment. Afterwards, also disable Fleet Management in the database using ` + "`call fleetManagement.disable();`" + `.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete the token of a deployment in the default project
neo4j-cli aura deployment token delete --deployment-id 22222222-2222-2222-2222-222222222222 --rw --yes --force

# Delete the token in a specific project
neo4j-cli aura deployment token delete --deployment-id 22222222-2222-2222-2222-222222222222 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			deploymentID := depflag.Get(cmd)
			scope := auraclient.Scope{OrgID: orgID, ProjectID: projectID}

			if err := confirm.Require(cmd, deploymentID); err != nil {
				return err
			}
			if err := auraclient.New(cfg).Deployments().DeleteToken(cmd.Context(), scope, deploymentID); err != nil {
				return err
			}
			cmd.Println("Deployment token deleted successfully for deployment", deploymentID)
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
