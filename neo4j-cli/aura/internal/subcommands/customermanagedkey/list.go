// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

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
		Short: "Returns a list of customer managed keys",
		Long: `This subcommand returns a list containing a summary of each of your customer managed keys. To find out more about a specific key, retrieve the details using the get subcommand.

You can filter keys in a particular tenant using --tenant-id. If the tenant flag is not specified, this endpoint lists all keys a user has access to across all tenants.`,
		Example: `# List all customer managed keys the current user has access to
neo4j-cli aura customer-managed-key list

# List keys in a specific tenant
neo4j-cli aura customer-managed-key list --tenant-id 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura customer-managed-key list --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/customer-managed-keys"
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
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id"})

			}

			return nil
		},
	}

	cmd.Flags().StringVar(&tenantId, "tenant-id", "", "An optional Tenant ID to filter customer managed keys in a tenant")

	return cmd
}
