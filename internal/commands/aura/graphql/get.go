// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get details of a GraphQL Data API",
		Long:  "This endpoint returns details of a specific GraphQL Data API.",
		Example: `# Get details of a GraphQL Data API (using flags)
neo4j-cli aura graphql get 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# Get details of a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql get 11111111 --instance-id 00000000

# Get details of a GraphQL Data API as JSON
neo4j-cli aura graphql get 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			graphqlId := strings.TrimSpace(args[0])
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}
			g, err := auraclient.New(cfg).GraphQL().Get(cmd.Context(), instanceId, graphqlId)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, g.Record, []string{"id", "name", "status", "url", "type_definitions"})
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to get the GraphQL Data API details for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
