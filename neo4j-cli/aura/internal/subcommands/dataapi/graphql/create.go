// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag       = "instance-id"
		nameFlag             = "name"
		instanceUsernameFlag = "instance-username"
		instancePasswordFlag = "instance-password"
		typeDefsFlag         = "type-definitions"
		typeDefsFileFlag     = "type-definitions-file"
		awaitFlag            = "await"
	)

	var (
		instanceId       string
		name             string
		instanceUsername string
		instancePassword string
		typeDefs         string
		typeDefsFile     string
		await            bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new GraphQL Data API",
		Long: `This command starts the creation process of a GraphQL Data API.

Creating a GraphQL Data API is an asynchronous operation. Use the --await flag to wait for the GraphQL Data API to be ready. Once the status transitions from "creating" to "ready" you may begin to use your GraphQL Data API.

This command returns your GraphQL Data API ID, API key, and connection URL for you to use once the GraphQL Data API is running. It is important to store the API key as it is not currently possible to get this or update it.

If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"name": name,
				"aura_instance": map[string]string{
					"username": instanceUsername,
					"password": instancePassword,
				},
				"security": map[string]any{
					"authentication_providers": []map[string]any{
						{
							"type":    "api-key",
							"name":    "default",
							"enabled": true,
						},
					},
				},
			}

			typeDefsForBody, err := GetTypeDefsFromFlag(cfg, typeDefs, typeDefsFile)
			if err != nil {
				return err
			}
			body["type_definitions"] = typeDefsForBody

			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/data-apis/graphql", instanceId)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: GraphQL Data API create should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {

				cmd.Println("###############################")
				cmd.Println("# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.")
				cmd.Println("###############################")

				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url", "authentication_providers"})

				if await {
					cmd.Println("Waiting for GraphQL Data API to be ready...")
					var response api.CreateGraphQLDataApiResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, response.Data.Id, api.GraphQLDataApiStatusCreating)
					if err != nil {
						return err
					}

					cmd.Println("GraphQL Data API Status:", pollResponse.Data.Status)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance to create the GraphQL Data API for")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&instanceUsername, instanceUsernameFlag, "", "(required) The username of the instance this GraphQL Data API will be connected to")
	cmd.MarkFlagRequired(instanceUsernameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&instancePassword, instancePasswordFlag, "", "(required) The password of the instance this GraphQL Data API will be connected to")
	cmd.MarkFlagRequired(instancePasswordFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the GraphQL Data API")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&typeDefs, typeDefsFlag, "", "The GraphQL type definitions, NOTE: must be base64 encoded")

	cmd.Flags().StringVar(&typeDefsFile, typeDefsFileFlag, "", "Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql. Must be of file type .graphql")
	cmd.MarkFlagsMutuallyExclusive(typeDefsFlag, typeDefsFileFlag)
	cmd.MarkFlagsOneRequired(typeDefsFlag, typeDefsFileFlag)

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until created GraphQL Data API is ready.")

	return cmd
}
