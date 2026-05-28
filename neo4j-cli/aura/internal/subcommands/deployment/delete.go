// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
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
		Use:         "delete",
		Short:       "Delete the given deployment",
		Long: `Deletes the given Fleet Manager deployment. This will only delete the deployment from Fleet Manager without affecting the actual running database. It is advised to disable Fleet Management for the database using ` + "`call fleetManagement.disable()`" + `.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete a deployment by ID
neo4j-cli aura deployment delete 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete a deployment in a specific organization and project
neo4j-cli aura deployment delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw --yes --force

# Delete a deployment and emit the response as JSON for scripting
neo4j-cli aura deployment delete 00000000-0000-0000-0000-000000000000 --rw --yes --force --format json`,
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

			deploymentId := strings.TrimSpace(args[0])

			if err := confirm.Require(cmd, deploymentId); err != nil {
				return err
			}

			path := fmt.Sprintf("/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId)
			_, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodDelete,
				Version: api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if api.IsSuccessful(statusCode) {
				fmt.Fprintf(cmd.ErrOrStderr(), "deployment %s deleted\n", deploymentId) //nolint:errcheck // narration to stderr; write errors are not actionable
				output.PrintBodyMap(cmd, cfg,
					api.NewSingleValueResponseData(map[string]any{"deleted": true, "id": deploymentId}),
					[]string{"deleted", "id"})
			}

			return nil
		},
	}

	confirm.Register(cmd)

	cmd.Flags().StringVar(&organizationId, organizationIdFlag, "", "(required) Organization ID")
	cmd.Flags().StringVar(&projectId, projectIdFlag, "", "(required) Project ID")

	return cmd
}
