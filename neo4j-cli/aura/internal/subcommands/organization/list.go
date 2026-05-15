// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package organization

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Returns a list of organizations",
		Long:  "This subcommand returns a list of Aura organizations accessible to the current user.",
		Example: `# List all organizations the current user has access to
neo4j-cli aura organization list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura organization list --format json

# Pipe organization ids through jq for a follow-up command
neo4j-cli aura organization list --format json | jq -r '.data[].id'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, "/organizations", &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion2,
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
