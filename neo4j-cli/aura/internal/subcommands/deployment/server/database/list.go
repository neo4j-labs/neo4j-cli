// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package serverdatabase

import (
	"fmt"
	"log"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
		deploymentId   string
		serverId       string
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
		deploymentIdFlag   = "deployment-id"
		serverIdFlag       = "server-id"
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns deployment server databases.",
		Long:  "Returns databases for the given Fleet Manager deployment server.",
		Example: `# List databases on a deployment server
neo4j-cli aura deployment server database list --deployment-id 00000000-0000-0000-0000-000000000000 --server-id 00000000-0000-0000-0000-000000000000

# List databases in a specific organization and project
neo4j-cli aura deployment server database list --deployment-id 00000000-0000-0000-0000-000000000000 --server-id 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# List databases as JSON for scripting
neo4j-cli aura deployment server database list --deployment-id 00000000-0000-0000-0000-000000000000 --server-id 00000000-0000-0000-0000-000000000000 --format json`,
		Args: cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers/%s/databases", organizationId, projectId, deploymentId, serverId)

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if api.IsSuccessful(statusCode) {
				fields := []string{
					"name",
					"type",
					"current_status",
					"last_committed_txn",
					"last_seen",
					"replication_lag",
					"role",
					"writer",
				}
				output.PrintBody(cmd, cfg, resBody, fields)
			}

			return nil
		},
	}
	cmd.Flags().StringVarP(&organizationId, organizationIdFlag, "o", "", "(required) Organization ID")
	cmd.Flags().StringVarP(&projectId, projectIdFlag, "p", "", "(required) Project/tenant ID")
	cmd.Flags().StringVarP(&deploymentId, deploymentIdFlag, "d", "", "(required) Deployment ID")
	cmd.Flags().StringVarP(&serverId, serverIdFlag, "s", "", "(required) Server ID")

	err := cmd.MarkFlagRequired(deploymentIdFlag)
	if err != nil {
		log.Fatal(err)
	}
	err = cmd.MarkFlagRequired(serverIdFlag)
	if err != nil {
		log.Fatal(err)
	}

	return cmd
}
