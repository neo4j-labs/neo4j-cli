// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var tenantId string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of instances",
		Long: `This subcommand returns a list containing a summary of each of your Aura instances. To find out more about a specific instance, retrieve the details using the get subcommand.

You can filter instances in a particular tenant using --tenant-id. If the tenant flag is not specified, this subcommand lists all instances a user has access to across all tenants.`,
		Example: `# List all instances the current user has access to
neo4j-cli aura instance list

# List instances in a specific tenant
neo4j-cli aura instance list --tenant-id 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura instance list --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/instances"

			queryParams := make(map[string]string)
			if tenantId != "" {
				queryParams["tenantId"] = tenantId
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
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "cloud_provider"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&tenantId, "tenant-id", "", "An optional Tenant ID to filter instances in a tenant")

	return cmd
}
