// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		dataApiId  string
	)

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get details of a GraphQL Data API authentication provider",
		Long:  "This endpoint returns details of a specific GraphQL Data API authentication provider.",
		Example: `# Get details of an authentication provider (using flags)
neo4j-cli aura graphql auth-provider get 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details of an authentication provider using a configured default workspace
neo4j-cli aura graphql auth-provider get 22222222 --instance-id 00000000 --data-api-id 11111111

# Get details of an authentication provider as JSON
neo4j-cli aura graphql auth-provider get 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			authProviderId := strings.TrimSpace(args[0])
			_, projectID, err := utils.ResolveAndValidateOrgProject(cmd, cfg)
			if err != nil {
				return err
			}
			if _, err = utils.FetchAndVerifyInstanceInProject(cfg, instanceId, projectID); err != nil {
				return err
			}
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{Method: http.MethodGet, Version: api.AuraApiVersionBeta1})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "type", "enabled", "url"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, "data-api-id", "", "(required) The ID of the GraphQL Data API to get the authentication provider of")
	cmd.MarkFlagRequired("data-api-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
