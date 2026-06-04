// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag     = "instance-id"
		nameFlag           = "name"
		serviceAccountFlag = "service-account"
		memoryFlag         = "memory"
		typeDefsFlag       = "type-definitions"
		typeDefsFileFlag   = "type-definitions-file"
	)

	var (
		instanceId     string
		name           string
		serviceAccount string
		memory         string
		typeDefs       string
		typeDefsFile   string
		wait           bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new GraphQL Data API",
		Long: `This command starts the creation process of a GraphQL Data API.

Creating a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "creating" to "ready" you may begin to use your GraphQL Data API.

This command returns your GraphQL Data API ID, API key, and connection URL for you to use once the GraphQL Data API is running. It is important to store the API key as it is not currently possible to get this or update it.

If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.`,
		Example: `# Create a GraphQL Data API from inline type definitions (base64-encoded)
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --type-definitions dHlwZSBNb3ZpZSB7IHRpdGxlOiBTdHJpbmcgfQ== --rw

# Create a GraphQL Data API without specifying a name (auto-generated)
neo4j-cli aura graphql create --instance-id 00000000 --memory 256MB --type-definitions-file ./typeDefs.graphql --rw

# Create a GraphQL Data API from a local type definitions file
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 512MB --type-definitions-file ./typeDefs.graphql --rw

# Create a GraphQL Data API with read-only service account
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --service-account read_only --type-definitions-file ./typeDefs.graphql --rw

# Create a GraphQL Data API and wait until it is ready
neo4j-cli aura graphql create --instance-id 00000000 --name my-api --memory 256MB --type-definitions-file ./typeDefs.graphql --wait --rw`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if serviceAccount != "read_only" && serviceAccount != "read_write" {
				return fmt.Errorf("invalid value for --service-account: %q, must be one of: read_only, read_write", serviceAccount)
			}

			switch memory {
			case "256MB", "512MB", "1024MB", "2048MB", "4096MB":
			default:
				return fmt.Errorf("invalid value for --memory: %q, must be one of: 256MB, 512MB, 1024MB, 2048MB, 4096MB", memory)
			}

			resolvedName, err := resolveGraphQLName(cfg, name, instanceId)
			if err != nil {
				return err
			}

			body := map[string]any{
				"name":   resolvedName,
				"memory": memory,
				"aura_instance": map[string]string{
					"service_account": serviceAccount,
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
				Version:  api.AuraApiVersionBeta1,
			})
			if err != nil {
				return err
			}

			// NOTE: GraphQL Data API create should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {

				fmt.Fprintln(cmd.ErrOrStderr(), "###############################")                                                                                                                                            //nolint:errcheck // narration to stderr; write errors are not actionable
				fmt.Fprintln(cmd.ErrOrStderr(), "# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.") //nolint:errcheck // narration to stderr; write errors are not actionable
				fmt.Fprintln(cmd.ErrOrStderr(), "###############################")                                                                                                                                            //nolint:errcheck // narration to stderr; write errors are not actionable

				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url", "authentication_providers"})

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
					var response api.CreateGraphQLDataApiResponse
					if err := json.Unmarshal(resBody, &response); err != nil {
						return err
					}

					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, response.Data.Id, api.GraphQLDataApiStatusCreating)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance to create the GraphQL Data API for")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&serviceAccount, serviceAccountFlag, "read_write", "The service account type for the instance connection, must be one of: read_only, read_write")

	cmd.Flags().StringVar(&memory, memoryFlag, "", "(required) Memory allocated to the GraphQL Data API, must be one of: 256MB, 512MB, 1024MB, 2048MB, 4096MB")
	cmd.MarkFlagRequired(memoryFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the GraphQL Data API (auto-generated if not specified)")

	cmd.Flags().StringVar(&typeDefs, typeDefsFlag, "", "The GraphQL type definitions, NOTE: must be base64 encoded")

	cmd.Flags().StringVar(&typeDefsFile, typeDefsFileFlag, "", "Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql. Must be of file type .graphql")
	cmd.MarkFlagsMutuallyExclusive(typeDefsFlag, typeDefsFileFlag)
	cmd.MarkFlagsOneRequired(typeDefsFlag, typeDefsFileFlag)

	flags.RegisterWait(cmd, &wait, "Waits until created GraphQL Data API is ready.")

	return cmd
}

// resolveGraphQLName returns the explicit name when non-empty, otherwise it
// lists the instance's GraphQL data APIs and derives an unused default name
// (e.g. GraphQL01). Shared by create's auto-naming path.
func resolveGraphQLName(cfg *clicfg.Config, name, instanceID string) (string, error) {
	if name != "" {
		return name, nil
	}

	listPath := fmt.Sprintf("/instances/%s/data-apis/graphql", instanceID)
	listBody, _, listErr := api.MakeRequest(cfg, listPath, &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersionBeta1,
	})
	if listErr != nil {
		return "", listErr
	}
	listData := api.ParseBody(listBody)
	existingNames := make([]string, 0, len(listData.AsArray()))
	for _, inst := range listData.AsArray() {
		if n, ok := inst["name"].(string); ok {
			existingNames = append(existingNames, n)
		}
	}
	return defaultGraphQLName(existingNames), nil
}
