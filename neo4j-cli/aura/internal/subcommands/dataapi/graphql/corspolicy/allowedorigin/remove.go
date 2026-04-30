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

func NewRemoveCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:   "remove <origin>",
		Short: "Removes an allowed origin from the CORS policy",
		Long: `This command removes an allowed origin from the Cross-Origin Resource Sharing (CORS) policy of a GraphQL Data API.

Updating the CORS policy of a GraphQL Data API is an asynchronous operation. Use the --await flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

Removing an allowed origin from the CORS policy of a GraphQL Data API means that most browsers are no longer able to make requests to the GraphQL Data API from a web app that is served from the specified origin.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			originToRemove := args[0]

			existingOrigins, err := getExistingOrigins(cfg, dataApiId, instanceId)
			if err != nil {
				return err
			}

			newOrigins := []string{}
			originFound := false

			for _, origin := range existingOrigins {
				if origin != originToRemove {
					newOrigins = append(newOrigins, origin)
				} else {
					originFound = true
				}
			}

			if !originFound {
				cmd.SilenceUsage = true
				return clierr.NewUsageError("Origin \"%s\" not found in allowed origins", originToRemove)
			}

			cmd.SilenceUsage = true
			body := map[string]any{
				"security": map[string]any{
					"cors_policy": map[string]any{
						"allowed_origins": newOrigins,
					},
				},
			}

			// TODO: theres currently a bug with the API that means you cannot send a body with only an empty array.
			// Therefore, as a temporary fix we add this dummy data that is ignored
			if len(newOrigins) == 0 {
				body["test"] = "ignore me"
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
				if len(newOrigins) == 0 {
					cmd.Println("New allowed origins: []")
				} else {
					cmd.Printf("New allowed origins: [\"%s\"]\n", strings.Join(newOrigins, "\", \""))
				}
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

	cmd.Flags().StringVar(&dataApiId, dataApiIdFlag, "", "(required) The ID of the GraphQL Data API to remove the CORS allowed origin for")
	cmd.MarkFlagRequired(dataApiIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until updated GraphQL Data API is ready.")

	return cmd
}
