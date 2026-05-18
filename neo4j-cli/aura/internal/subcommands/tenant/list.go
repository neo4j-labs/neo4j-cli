// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tenant

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Hidden: true,
		Use:    "list",
		Short:  "Returns a list of tenants",
		Long:   "This subcommand returns a list containing a summary of each of your Aura Tenants. To find out more about a specific Tenant, retrieve the details using the get subcommand.",
		Example: `# List all tenants the current user has access to
neo4j-cli aura tenant list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura tenant list --format json

# Pipe tenant ids through jq for a follow-up command
neo4j-cli aura tenant list --format json | jq -r '.data[].id'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: 'aura tenant list' is deprecated and will be removed in a future release. Use 'aura project list' instead.") //nolint:errcheck // deprecation warning to stderr; write errors are not actionable
			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, "/tenants", &api.RequestConfig{
				Method: http.MethodGet,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name"})
			}

			return nil
		},
	}
}
