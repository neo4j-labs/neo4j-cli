// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func newAllowedConfigsCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "allowed-configs",
		Short: "Returns the memory configurations selectable for a virtual graph",
		Long: `This subcommand returns the memory configurations selectable for virtual graphs in the specified project, and the default applied when --memory is omitted on create.

Use the returned memory values with the --memory flag of the create and update subcommands.`,
		Example: `# List the selectable virtual graph configurations for a project
neo4j-cli aura virtual-graph allowed-configs --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List the configurations using a configured default workspace
neo4j-cli aura virtual-graph allowed-configs

# Emit JSON for scripting and extract the selectable memory values with jq
neo4j-cli aura virtual-graph allowed-configs --format json | jq -r '.data.configs[].memory'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			orgID, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			path := api.ScopedVirtualGraphAllowedConfigsPath(orgID, projectID)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:  http.MethodGet,
				Version: api.AuraApiVersion2,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"default_memory", "configs"})
			}
			return nil
		},
	}
}
