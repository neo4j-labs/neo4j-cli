// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(cfg *clicfg.Config) *cobra.Command {
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
		Use:         "remove <origin>",
		Short:       "Removes an allowed origin from the CORS policy",
		Long: `This command removes an allowed origin from the Cross-Origin Resource Sharing (CORS) policy of a GraphQL Data API.

Updating the CORS policy of a GraphQL Data API is an asynchronous operation. Use the --wait flag to wait for the GraphQL Data API to be ready. Once the status transitions from "updating" to "ready" you may begin to use your GraphQL Data API.

Removing an allowed origin from the CORS policy of a GraphQL Data API means that most browsers are no longer able to make requests to the GraphQL Data API from a web app that is served from the specified origin.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Remove an allowed origin from the CORS policy
neo4j-cli aura data-api graphql cors-policy allowed-origin remove https://app.example.com --instance-id 00000000 --data-api-id 11111111 --rw --yes --force

# Remove an allowed origin and wait until the GraphQL Data API is ready
neo4j-cli aura data-api graphql cors-policy allowed-origin remove https://app.example.com --instance-id 00000000 --data-api-id 11111111 --wait --rw --yes --force

# Remove an allowed origin and capture the response as JSON
neo4j-cli aura data-api graphql cors-policy allowed-origin remove https://app.example.com --instance-id 00000000 --data-api-id 11111111 --rw --yes --force --format json`,
		Args: cobra.ExactArgs(1),
		RunE: nil,
	}

	confirmFlags := confirm.Register(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		originToRemove := strings.TrimSpace(args[0])

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

		if err := confirmFlags.Require(cmd, originToRemove); err != nil {
			if errors.Is(err, confirm.ErrCancelled) {
				fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.") //nolint:errcheck // narration to stderr; write errors are not actionable
				return nil
			}
			return err
		}

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
				fmt.Fprintln(cmd.ErrOrStderr(), "New allowed origins: []") //nolint:errcheck // narration to stderr; write errors are not actionable
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "New allowed origins: [\"%s\"]\n", strings.Join(newOrigins, "\", \"")) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
			output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})
			if wait {
				fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for GraphQL Data API to be ready...") //nolint:errcheck // narration to stderr; write errors are not actionable
				pollResponse, err := api.PollGraphQLDataApi(cfg, instanceId, dataApiId, api.GraphQLDataApiStatusUpdating)
				if err != nil {
					return err
				}

				fmt.Fprintln(cmd.ErrOrStderr(), "GraphQL Data API Status:", pollResponse.Data.Status) //nolint:errcheck // narration to stderr; write errors are not actionable
			}
		}
		return nil
	}

	cmd.Flags().StringVar(&instanceId, instanceIdFlag, "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired(instanceIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, dataApiIdFlag, "", "(required) The ID of the GraphQL Data API to remove the CORS allowed origin for")
	cmd.MarkFlagRequired(dataApiIdFlag) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	flags.RegisterWait(cmd, &wait, "Waits until updated GraphQL Data API is ready.")

	return cmd
}
