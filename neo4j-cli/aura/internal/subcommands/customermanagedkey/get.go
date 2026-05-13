// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Returns a customer managed key details",
		Long:  `This subcommand returns details about a specific Customer Managed Key.`,
		Example: `# Get details of a customer managed key by ID
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000

# Get details and emit JSON for scripting
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the key status
neo4j-cli aura customer-managed-key get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.status'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := fmt.Sprintf("/customer-managed-keys/%s", args[0])
			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodGet,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "tenant_id", "status", "created", "cloud_provider", "key_id", "region", "type"})

			}

			return nil
		},
	}
}
