// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
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
		instanceIdFlag = "instance-id"
		dataApiIdFlag  = "data-api-id"
		typeFlag       = "type"
		nameFlag       = "name"
		disabledFlag   = "disabled"
		urlFlag        = "url"
		awaitFlag      = "await"

		disabledDefault = false
	)

	var (
		instanceId string
		dataApiId  string
		_type      flags.AuthProviderType
		name       string
		disabled   bool
		url        string
		await      bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Creates a new GraphQL Data API authentication provider",
		Long: `This command creates a new GraphQL Data API authentication provider.

Creating a GraphQL Data API authentication provider is an asynchronous operation. Use the --await flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

If you create an 'api-key' Authentication provider, an API key will be created. It is important to store the API key as it is not currently possible to get it or update it.

If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if _type == api.GraphQLDataApiAuthProviderTypeJwks {
				cmd.MarkFlagRequired(urlFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
			}

			if _type == api.GraphQLDataApiAuthProviderTypeApiKey && url != "" {
				return fmt.Errorf("url flag can not be set for authentication provider type '%s'", api.GraphQLDataApiAuthProviderTypeApiKey)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"type":    _type,
				"name":    name,
				"enabled": !disabled,
			}

			if url != "" {
				body["url"] = url
			}

			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/auth-providers", instanceId, dataApiId)
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				PostBody: body,
				Method:   http.MethodPost,
			})
			if err != nil {
				return err
			}

			// NOTE: Auth provider create should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {

				if _type == api.GraphQLDataApiAuthProviderTypeApiKey {
					cmd.Println("###############################")
					cmd.Println("# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.")
					cmd.Println("###############################")
				}

				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "type", "enabled", "key", "url"})

				if await {
					cmd.Println("Waiting for GraphQL Data API to be ready...")
					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, dataApiId, api.GraphQLDataApiStatusCreating)
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

	cmd.Flags().StringVar(&dataApiId, dataApiIdFlag, "", "(required) The ID of the GraphQL Data API to create the authentication provider for")
	cmd.MarkFlagRequired(dataApiIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	msgTypeFlag := fmt.Sprintf("(required) The type of the Authentication provider, one of '%s' or '%s'", api.GraphQLDataApiAuthProviderTypeApiKey, api.GraphQLDataApiAuthProviderTypeJwks)
	cmd.Flags().Var(&_type, typeFlag, msgTypeFlag)
	cmd.MarkFlagRequired(typeFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) The name of the Authentication provider")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().BoolVar(&disabled, disabledFlag, disabledDefault, "Whether or not the Authentication provider is disabled")

	msgUrlFlag := fmt.Sprintf("The JWKS URL that you want the bearer tokens in incoming GraphQL requests to be validated against. NOTE: only applicable for Authentication provider type '%s'", api.GraphQLDataApiAuthProviderTypeJwks)
	cmd.Flags().StringVar(&url, urlFlag, "", msgUrlFlag)

	cmd.Flags().BoolVar(&await, awaitFlag, false, "Waits until created Authentication provider is ready.")

	return cmd
}
