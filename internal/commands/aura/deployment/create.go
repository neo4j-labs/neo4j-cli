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

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var name, connectionURL string

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Create a new deployment",
		Long: `Creates a new unmonitored Fleet Manager deployment and returns its ID.

Next, create a token with 'aura deployment token create' and register it in the database with ` + "`call fleetManagement.registerToken('$token');`" + `.`,
		Example: `# Create a deployment in the default project
neo4j-cli aura deployment create --name my-deployment --rw

# Create a deployment with a connection URL in a specific project
neo4j-cli aura deployment create --name my-deployment --connection-url neo4j+s://db.example.com:7687 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Create a deployment and emit JSON for scripting
neo4j-cli aura deployment create --name my-deployment --rw --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			d, err := auraclient.New(cfg).Deployments().Create(cmd.Context(), auraclient.Scope{OrgID: orgID, ProjectID: projectID}, name, connectionURL)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, d.Record, []string{"id"})
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "(required) Deployment name")
	cmd.Flags().StringVar(&connectionURL, "connection-url", "", "An optional connection URL for the deployment")
	cmd.MarkFlagRequired("name") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
