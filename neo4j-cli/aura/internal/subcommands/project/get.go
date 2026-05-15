// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config) *cobra.Command {
	var organizationId string

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Returns project details",
		Long:  "This subcommand returns details about a specific Aura project.",
		Example: `# Get project details by ID
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000

# Emit JSON for scripting
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the project name
neo4j-cli aura project get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.name'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, fmt.Sprintf("/tenants/%s", projectID), &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion1,
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

	// --organization-id is accepted for forward compatibility but not required and not used.
	cmd.Flags().StringVar(&organizationId, "organization-id", "", "Organization ID (accepted for forward compatibility; not used in the API call)")

	return cmd
}
