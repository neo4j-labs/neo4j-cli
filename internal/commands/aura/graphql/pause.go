// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"github.com/neo4j/cli/internal/aura"
	"strings"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/aura/output"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonflags "github.com/neo4j/cli/internal/flags"
	"github.com/spf13/cobra"
)

func NewPauseCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		wait       bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "pause <id>",
		Short:       "Pause a GraphQL Data API",
		Long: `This command starts the pausing process of an existing GraphQL Data API.

Pausing a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be paused. The GraphQL Data API will only be paused once the status transitions from "pausing" to "paused".`,
		Example: `# Pause a GraphQL Data API (using flags)
neo4j-cli aura graphql pause 11111111 --instance-id 00000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Pause a GraphQL Data API using a configured default workspace
neo4j-cli aura graphql pause 11111111 --instance-id 00000000 --rw

# Pause a GraphQL Data API and wait until it is paused
neo4j-cli aura graphql pause 11111111 --instance-id 00000000 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			graphqlId := strings.TrimSpace(args[0])
			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}
			g, err := aura.New(cfg).GraphQL().Pause(cmd.Context(), instanceId, graphqlId)
			if err != nil {
				return err
			}
			output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(g.Record), []string{"id", "name", "status", "url"})

			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be paused...") //nolint:errcheck // narration to stderr; write errors are not actionable
				status, err := aura.New(cfg).GraphQL().WaitWhile(cmd.Context(), instanceId, graphqlId, aura.GraphQLStatusPausing)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to pause the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	commonflags.RegisterWait(cmd, &wait, "Waits until GraphQL Data API is paused.")

	return cmd
}
