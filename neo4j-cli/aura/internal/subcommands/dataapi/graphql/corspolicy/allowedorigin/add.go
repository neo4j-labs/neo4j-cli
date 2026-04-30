// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewAddCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag = "instance-id"
		dataApiIdFlag  = "data-api-id"
		awaitFlag      = "await"
	)

	var (
		instanceId string
		dataApiId  string
		await      bool
	)

	cmd := &cobra.Command{
		Use:   "add <origin>",
		Short: "Adds a new allowed origin to the CORS policy",
		Long: `This command adds a new allowed origin to the Cross-Origin Resource Sharing (CORS) policy of a GraphQL Data API.

Updating the CORS policy of a GraphQL Data API is an asynchronous operation. Use the --await flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

Adding a new allowed origin to the CORS policy of a GraphQL Data API allows browsers to make requests to the GraphQL Data API from a web app that is served from the specified origin.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			newOrigin := args[0]

			existingOrigins, err := getExistingOrigins(cfg, dataApiId, instanceId)
			if err != nil {
				return err
			}

			for _, origin := range existingOrigins {
				if origin == newOrigin {
					cmd.SilenceUsage = true
					return clierr.NewUsageError("Origin \"%s\" already exists in allowed origins", newOrigin)
				}
			}

			newOrigins := append(existingOrigins, newOrigin)

			cmd.SilenceUsage = true
			body := map[string]any{
				"security": map[string]any{
					"cors_policy": map[string]any{
						"allowed_origins": newOrigins,
					},
				},
			}
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s", instanceId, dataApiId)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPatch,
			})
			if err != nil {
				return err
			}

			// NOTE: Update should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				cmd.Printf("New allowed origins: [\"%s\"]\n", strings.Join(newOrigins, "\", \""))
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})
				if await {
					cmd.Println("Waiting for GraphQL Data API to be ready...")
					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, dataApiId, api.GraphQLDataApiStatusUpdating)
					if err != nil {
						return err
					}

					cmd.Println("GraphQL Data API Status:", pollResponse.Data.Status)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, dataApiIdFlag, "", "(required) The ID of the GraphQL Data API to add the CORS allowed origin for")
	cmd.MarkFlagRequired(dataApiIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until updated GraphQL Data API is ready.")

	return cmd
}
