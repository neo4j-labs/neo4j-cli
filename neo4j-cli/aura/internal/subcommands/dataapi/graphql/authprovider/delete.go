// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		dataApiId  string
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Delete a GraphQL Data API authentication provider",
		Long: `Deletes a GraphQL Data API authentication provider. This action can not be undone.

Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.`,
		Example: `# Delete an authentication provider
neo4j-cli aura data-api graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --rw --yes --force

# Delete an authentication provider and capture the response as JSON
neo4j-cli aura data-api graphql auth-provider delete 22222222 --instance-id 00000000 --data-api-id 11111111 --rw --yes --force --format json

# Delete the first authentication provider returned by list
neo4j-cli aura data-api graphql auth-provider delete $(neo4j-cli aura data-api graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --format json | jq -r '.data[0].id') --instance-id 00000000 --data-api-id 11111111 --rw --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			authProviderId := strings.TrimSpace(args[0])

			if err := confirm.Require(cmd, authProviderId); err != nil {
				return err
			}

			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/auth-providers/%s", instanceId, dataApiId, authProviderId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodDelete,
			})
			if err != nil {
				return err
			}

			// NOTE: delete should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "type", "enabled", "url"})
			}
			return nil
		},
	}

	confirm.Register(cmd)

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to delete the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, "data-api-id", "", "(required) The ID of the GraphQL Data API to delete the Authentication provider for")
	cmd.MarkFlagRequired("data-api-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
