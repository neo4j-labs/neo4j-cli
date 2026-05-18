// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of Graph Analytics Serverless sessions",
		Example: `# List all Graph Analytics sessions in a project
neo4j-cli aura graph-analytics session list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List sessions using a configured default workspace
neo4j-cli aura graph-analytics session list

# List sessions attached to a specific instance and emit JSON for scripting
neo4j-cli aura graph-analytics session list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --instance-id 00000000 --format json`,
		Long: `This subcommand returns a list containing a summary of each of your Graph Analytics Serverless sessions in the specified project.

Use --organization-id and --project-id to specify which project's sessions to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			path := "/graph-analytics/sessions"

			queryParams := map[string]string{
				"tenantId": projectID,
			}
			if instanceId != "" {
				queryParams["instanceId"] = instanceId
			}

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:      http.MethodGet,
				QueryParams: queryParams,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				renamed := utils.RenameResponseField(responseData, "tenant_id", "project_id")
				output.PrintBodyMap(cmd, cfg, renamed, []string{"id", "name", "status", "project_id", "cloud_provider", "ttl"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "An optional Instance ID to filter for sessions attached to an instance")

	return cmd
}
