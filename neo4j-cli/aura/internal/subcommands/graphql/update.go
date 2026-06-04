// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	const (
		instanceIdFlag     = "instance-id"
		nameFlag           = "name"
		serviceAccountFlag = "service-account"
		typeDefsFlag       = "type-definitions"
		typeDefsFileFlag   = "type-definitions-file"
	)

	var (
		instanceId     string
		name           string
		serviceAccount string
		typeDefs       string
		typeDefsFile   string
		wait           bool
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Edit a GraphQL Data API",
		Long: `This endpoint edits a specific GraphQL Data API.

Updating a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready again. Once the status transitions from "updating" to "ready" you may continue to use your GraphQL Data API.`,
		Example: `# Rename a GraphQL Data API
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --name renamed-api --rw

# Update the service account permission and wait for the API to be ready
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --service-account read_only --wait --rw

# Replace the type definitions from a local file
neo4j-cli aura graphql update 11111111 --instance-id 00000000 --type-definitions-file ./typeDefs.graphql --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			graphqlId := strings.TrimSpace(args[0])

			if serviceAccount != "" && serviceAccount != "read_only" && serviceAccount != "read_write" {
				return fmt.Errorf("invalid --service-account value %q: must be read_only or read_write", serviceAccount)
			}

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

			if serviceAccount != "" {
				body["aura_instance"] = map[string]string{"service_account": serviceAccount}
			}

			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s", instanceId, graphqlId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method:   http.MethodPatch,
				PostBody: body,
				Version:  api.AuraApiVersionBeta1,
			})
			if err != nil {
				return err
			}

			// NOTE: GraphQL Data API update should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})

				if wait {
					fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be updated...") //nolint:errcheck // narration to stderr; write errors are not actionable
					pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, graphqlId, api.GraphQLDataApiStatusUpdating)
					if err != nil {
						return err
					}

					fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance to update the Data API for")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&name, nameFlag, "", "The name of the GraphQL Data API")

	cmd.Flags().StringVar(&serviceAccount, serviceAccountFlag, "", "The service account permission for the instance this GraphQL Data API will be connected to (read_only or read_write)")

	cmd.Flags().StringVar(&typeDefs, typeDefsFlag, "", "The GraphQL type definitions, NOTE: must be base64 encoded")

	cmd.Flags().StringVar(&typeDefsFile, typeDefsFileFlag, "", "Path to a local GraphQL type definitions file, e.g. path/to/typeDefs.graphql")
	cmd.MarkFlagsMutuallyExclusive(typeDefsFlag, typeDefsFileFlag)

	flags.RegisterWait(cmd, &wait, "Waits until updated GraphQL Data API is ready again.")

	return cmd
}
