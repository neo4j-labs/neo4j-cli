// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of instances",
		Long: `This subcommand returns a list containing a summary of each of your Aura instances in the specified project. To find out more about a specific instance, retrieve the details using the get subcommand.

Use --organization-id and --project-id to specify which project's instances to list, or configure a default with 'aura workspace use <org-id>/<project-id>'.`,
		Example: `# List instances in a project (using flags)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List instances using a configured default workspace
neo4j-cli aura instance list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			path := api.ScopedInstancesPath(orgID, projectID)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				normalized := utils.NormalizeV2Beta1Response(responseData)
				// Inject the owning organization so downstream consumers
				// (e.g. neo4j_cli_list_targets) can surface the relationship.
				rows := normalized.AsArray()
				for i := range rows {
					if _, ok := rows[i]["organization_id"]; !ok {
						rows[i]["organization_id"] = orgID
					}
				}
				normalized = api.NewListResponseData(rows)
				output.PrintBodyMap(cmd, cfg, normalized, []string{"id", "name", "status", "organization_id", "project_id", "cloud_provider"})
			}
			return nil
		},
	}

	return cmd
}
