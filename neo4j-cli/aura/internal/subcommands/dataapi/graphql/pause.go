// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewPauseCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		await      bool
	)

	cmd := &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause a GraphQL Data API",
		Long: `This command starts the pausing process of an existing GraphQL Data API.

Pausing a GraphQL Data API is an asynchronous operation. Use the --await flag to wait for the GraphQL Data API to be paused. The GraphQL Data API will only be paused once the status transitions from "pausing" to "paused".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/pause", instanceId, args[0])

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: pause should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})

				if await {
					cmd.Println("Waiting for GraphQL Data API to be paused...")
					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, args[0], api.GraphQLDataApiStatusPausing)
					if err != nil {
						return err
					}

					cmd.Println("GraphQL Data API Status:", pollResponse.Data.Status)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to pause the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().BoolVar(&await, "await", false, "Waits until GraphQL Data API is paused.")

	return cmd
}
