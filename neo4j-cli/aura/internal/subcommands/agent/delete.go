// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		organizationId string
		projectId      string
	)

	const (
		organizationIdFlag = "organization-id"
		projectIdFlag      = "project-id"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Deletes an agent",
		Long:        "Deletes an agent by its ID.",
		Example: `# Delete an agent by ID
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw

# Delete an agent in a specific organization and project
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Delete an agent and emit the response as JSON
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --format json`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			agentId := args[0]
			path := fmt.Sprintf("/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId)

			cmd.SilenceUsage = true
			_, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodDelete,
				Version: api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if api.IsSuccessful(statusCode) {
				cmd.Println("Agent deleted successfully", agentId)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project/tenant ID")

	return cmd
}
