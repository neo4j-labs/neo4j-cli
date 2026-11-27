// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token

import (
	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/deployment/internal/depflag"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Create a deployment token",
		Long:        `Creates a new auto-rotating Fleet Manager deployment token with a three month rotation interval. Register the deployment with Fleet Manager using the ` + "`call fleetManagement.registerToken('$token');`" + ` database procedure.`,
		Example: `# Create a token for a deployment in the default project
neo4j-cli aura deployment token create --deployment-id 22222222-2222-2222-2222-222222222222 --rw

# Create a token in a specific project and emit JSON
neo4j-cli aura deployment token create --deployment-id 22222222-2222-2222-2222-222222222222 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			deploymentID := depflag.Get(cmd)
			scope := auraclient.Scope{OrgID: orgID, ProjectID: projectID}

			rec, err := auraclient.New(cfg).Deployments().CreateToken(cmd.Context(), scope, deploymentID)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, rec, []string{"token"})
			return nil
		},
	}

	return cmd
}
