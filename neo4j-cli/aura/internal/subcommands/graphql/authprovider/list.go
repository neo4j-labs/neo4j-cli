// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		instanceId string
		dataApiId  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns a list of authentication providers of a specific GraphQL Data API",
		Example: `# List authentication providers of a GraphQL Data API
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111

# List authentication providers as JSON for scripting
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --format json

# Show only enabled provider names
neo4j-cli aura graphql auth-provider list --instance-id 00000000 --data-api-id 11111111 --format json | jq -r '.data[] | select(.enabled) | .name'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/auth-providers", instanceId, dataApiId)

			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{Method: http.MethodGet, Version: api.AuraApiVersionBeta1})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				output.PrintBody(cmd, cfg, resBody, []string{"id", "name", "type", "enabled", "url"})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&instanceId, "instance-id", "", "(required) The ID of the instance the GraphQL Data API is connected to")
	cmd.MarkFlagRequired("instance-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	cmd.Flags().StringVar(&dataApiId, "data-api-id", "", "(required) The ID of the GraphQL Data API to list the authentication providers of")
	cmd.MarkFlagRequired("data-api-id") //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup

	return cmd
}
