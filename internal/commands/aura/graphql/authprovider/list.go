// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"github.com/neo4j/cli/internal/aura"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		dataApiId  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of authentication providers of a specific GraphQL Data API",
		Example: `# List authentication providers of a GraphQL Data API (using flags)
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111

# List authentication providers using a configured default workspace
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111

# List authentication providers as JSON for scripting
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}
			providers, err := aura.New(cfg).GraphQL().AuthProviders().List(cmd.Context(), instanceId, dataApiId)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(providers))
			for i, p := range providers {
				rows[i] = p.Record
			}
			output.PrintRecords(cmd, cfg, rows, []string{"id", "name", "type", "enabled", "url"})
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, "data-api-id", "", "(required) The ID of the GraphQL Data API to list the authentication providers of")
	cmd.MarkFlagRequired("data-api-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
