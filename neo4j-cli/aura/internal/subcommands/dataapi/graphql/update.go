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

func NewUpdateCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:   "update <id>",
		Short: "Edit a GraphQL Data API",
		Long: `This endpoint edits a specific GraphQL Data API.
		
Updating a GraphQL Data API is an asynchronous operation. Use the --await flag to wait for the GraphQL Data API to be ready again. Once the status transitions from "updating" to "ready" you may continue to use your GraphQL Data API.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}

			if name != "" {
				body["name"] = name
			}

			if typeDefs != "" || typeDefsFile != "" {
				base64EncodedTypeDefs, err := GetTypeDefsFromFlag(cfg, typeDefs, typeDefsFile)
				if err != nil {
					return err
				}
				body["type_definitions"] = base64EncodedTypeDefs
			}

			if instanceUsername != "" || instancePassword != "" {
				auraInstance := map[string]string{}

				if instanceUsername != "" {
					auraInstance["username"] = instanceUsername
				}
				if instancePassword != "" {
					auraInstance["password"] = instancePassword
				}

				body["aura_instance"] = auraInstance
			}

			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s", instanceId, args[0])

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPatch,
				PostBody: body,
			})
			if err != nil {
				return err
			}

			// NOTE: GraphQL Data API update should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})

				if await {
					cmd.Println("Waiting for GraphQL Data API to be updated...")
					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, args[0], api.GraphQLDataApiStatusUpdating)
					if err != nil {
						return err
					}

					cmd.Println("GraphQL Data API Status:", pollResponse.Data.Status)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance to update the Data API for")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the GraphQL Data API")

	cmd.Flags().StringVar(&instanceUsername, instanceUsernameFlag, "", "The username of the instance this GraphQL Data API will be connected to")

	cmd.Flags().StringVar(&instancePassword, instancePasswordFlag, "", "The password of the instance this GraphQL Data API will be connected to")

	cmd.Flags().StringVar(&typeDefs, typeDefsFlag, "", "The GraphQL type definitions, NOTE: must be base64 encoded")

	cmd.Flags().StringVar(&typeDefsFile, typeDefsFileFlag, "", "Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql")
	cmd.MarkFlagsMutuallyExclusive(typeDefsFlag, typeDefsFileFlag)

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until updated GraphQL Data API is ready again.")

	return cmd
}
