// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	"github.com/neo4j/cli/internal/confirm"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Delete a GraphQL Data API",
		Long: `Deletes a GraphQL Data API. This action can not be undone.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete a GraphQL Data API (using flags)
neo4j-cli aura graphql delete 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force

# Delete a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql delete 11111111 --instance-id 00000000 --rw --yes --force

# Delete a GraphQL Data API and capture the response as JSON
neo4j-cli aura graphql delete 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw --yes --force --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			graphqlId := strings.TrimSpace(args[0])

			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			if err := confirm.Require(cmd, graphqlId); err != nil {
				return err
			}

			g, err := aura.New(cfg).GraphQL().Delete(cmd.Context(), instanceId, graphqlId)
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(g.Record), []string{"id", "name", "status", "url"})
			return nil
		},
	}

	confirm.Register(cmd)

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to delete the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
