// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin

import (
	"fmt"
	"github.com/neo4j/cli/internal/auraclient"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonflags "github.com/neo4j/cli/internal/flags"
	"github.com/spf13/cobra"
)

func NewAddCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag = "instance-id"
		dataApiIdFlag  = "data-api-id"
	)

	var (
		instanceId string
		dataApiId  string
		wait       bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "add <origin>",
		Short:       "Adds a new allowed origin to the CORS policy",
		Long: `This command adds a new allowed origin to the Cross-Origin Resource Sharing (CORS) policy of a GraphQL Data API.

Updating the CORS policy of a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

Adding a new allowed origin to the CORS policy of a GraphQL Data API allows browsers to make requests to the GraphQL Data API from a web app that is served from the specified origin.`,
		Example: `# Add an allowed origin to the CORS policy (using flags)
neo4j-cli aura graphql cors-policy allowed-origin add https://app.example.com --instance-id 00000000 --data-api-id 11111111 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw

# Add an allowed origin using a configured default workspace
neo4j-cli aura graphql cors-policy allowed-origin add https://app.example.com --instance-id 00000000 --data-api-id 11111111 --rw

# Add an allowed origin and wait until the GraphQL Data API is ready
neo4j-cli aura graphql cors-policy allowed-origin add https://app.example.com --instance-id 00000000 --data-api-id 11111111 --wait --organization-id 00000000-0000-0000-0000-000000000000 --project-id 11111111-1111-1111-1111-111111111111 --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			newOrigin := strings.TrimSpace(args[0])

			if _, err := utils.ResolveAndVerifyInstance(cmd, cfg, instanceId); err != nil {
				return err
			}

			existingOrigins, err := auraclient.New(cfg).GraphQL().AllowedOrigins(cmd.Context(), instanceId, dataApiId)
			if err != nil {
				return err
			}

			for _, origin := range existingOrigins {
				if origin == newOrigin {
					return clierr.NewUsageError("Origin \"%s\" already exists in allowed origins", newOrigin)
				}
			}

			newOrigins := append(existingOrigins, newOrigin)

			g, err := auraclient.New(cfg).GraphQL().SetAllowedOrigins(cmd.Context(), instanceId, dataApiId, newOrigins)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "New allowed origins: [\"%s\"]\n", strings.Join(newOrigins, "\", \"")) //nolint:errcheck // narration to stderr; write errors are not actionable
			output.PrintRecord(cmd, cfg, g.Record, []string{"id", "name", "status", "url"})
			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
				status, err := auraclient.New(cfg).GraphQL().WaitWhile(cmd.Context(), instanceId, dataApiId, auraclient.GraphQLStatusUpdating)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, dataApiIdFlag, "", "(required) The ID of the GraphQL Data API to add the CORS allowed origin for")
	cmd.MarkFlagRequired(dataApiIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	commonflags.RegisterWait(cmd, &wait, "Waits until updated GraphQL Data API is ready.")

	return cmd
}
