// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		dataApiId  string
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Delete a GraphQL Data API authentication provider",
		Long: `Deletes a GraphQL Data API authentication provider. This action can not be undone.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete an authentication provider (using flags)
neo4j-cli aura graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete an authentication provider using a configured default workspace
neo4j-cli aura graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --rw --yes --force

# Delete an authentication provider and capture the response as JSON
neo4j-cli aura graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			authProviderId := strings.TrimSpace(args[0])

			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			if err := confirm.Require(cmd, authProviderId); err != nil {
				return err
			}

			p, err := aura.New(cfg).GraphQL().AuthProviders().Delete(cmd.Context(), instanceId, dataApiId, authProviderId)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, p.Record, []string{"id", "name", "type", "enabled", "url"})
			return nil
		},
	}

	confirm.Register(cmd)

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to delete the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, "data-api-id", "", "(required) The ID of the GraphQL Data API to delete the Authentication provider for")
	cmd.MarkFlagRequired("data-api-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
