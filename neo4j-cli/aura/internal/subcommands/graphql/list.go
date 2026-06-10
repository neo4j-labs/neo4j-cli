// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
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
		Short: "Returns a list of GraphQL Data APIs",
		Example: `# List GraphQL Data APIs of an instance (using flags)
neo4j-cli aura graphql list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List GraphQL Data APIs using a configured default workspace
neo4j-cli aura graphql list --instance-id 00000000

# List GraphQL Data APIs as JSON for scripting
neo4j-cli aura graphql list --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			if _, err = utils.FetchAndVerifyInstanceInProject(cfg, instanceId, projectID); err != nil {
				return err
			}
			path := fmt.Sprintf("/instances/%s/data-apis/graphql", instanceId)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{Method: http.MethodGet, Version: api.AuraApiVersionBeta1})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to list the GraphQL Data APIs of")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
