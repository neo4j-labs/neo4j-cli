// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newDeleteCmd(cfg *clicfg.Config) *cobra.Command {
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
		Long: `Deletes an agent by its ID.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete an agent by ID
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete an agent in a specific organization and project
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete an agent and emit the response as JSON
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --yes --force --format json`,
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return utils.SetProjectFlagsAsRequired(cfg, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			organizationId, projectId, err := utils.SetProjetDefaults(cfg, organizationId, projectId)
			if err != nil {
				return err
			}

			agentId := args[0]

			if err := confirm.Require(cmd, agentId); err != nil {
				return err
			}

			path := fmt.Sprintf("/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId)
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

	confirm.Register(cmd)

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "Project/tenant ID")

	return cmd
}
