// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	var instanceId string

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Delete a GraphQL Data API",
		Long:        "Deletes a GraphQL Data API. This action can not be undone.",
		Example: `# Delete a GraphQL Data API
neo4j-cli aura data-api graphql delete 11111111 --instance-id 00000000 --rw

# Delete a GraphQL Data API and capture the response as JSON
neo4j-cli aura data-api graphql delete 11111111 --instance-id 00000000 --rw --format json

# Delete a GraphQL Data API discovered via list
neo4j-cli aura data-api graphql delete $(neo4j-cli aura data-api graphql list --instance-id 00000000 --format json | jq -r '.data[0].id') --instance-id 00000000 --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			graphqlId := strings.TrimSpace(args[0])
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s", instanceId, graphqlId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodDelete,
			})
			if err != nil {
				return err
			}

			// NOTE: delete should not return OK (200), it always returns 202, checking both just in case
			if statusCode == http.StatusAccepted || statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "status", "url"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance to delete the Data API for")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
