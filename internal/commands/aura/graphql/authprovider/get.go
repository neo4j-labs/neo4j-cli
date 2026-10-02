// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
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
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}
			p, err := aura.New(cfg).GraphQL().AuthProviders().Get(cmd.Context(), instanceId, dataApiId, authProviderId)
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(p.Record), []string{"id", "name", "type", "enabled", "url"})
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, "data-api-id", "", "(required) The ID of the GraphQL Data API to get the authentication provider of")
	cmd.MarkFlagRequired("data-api-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
