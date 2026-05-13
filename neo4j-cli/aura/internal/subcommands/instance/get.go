// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Returns instance details",
		Long:  "This endpoint returns details about a specific Aura Instance.",
		Example: `# Get details of an instance by ID
neo4j-cli aura instance get 00000000-0000-0000-0000-000000000000

# Get details and emit JSON for scripting
neo4j-cli aura instance get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the connection URL
neo4j-cli aura instance get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.connection_url'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instanceId := args[0]
			path := fmt.Sprintf("/instances/%s", instanceId)

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodGet,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				fields, err := getFields(resBody)
				if err != nil {
					return err
				}
				output.PrintBody(cmd, cfg, resBody, fields)
			}

			return nil
		},
	}
}

func getFields(resBody []byte) ([]string, error) {
	responseBody := api.ParseBody(resBody)

	fields := []string{"id", "name", "tenant_id", "status", "connection_url", "cloud_provider", "region", "type", "memory", "storage", "customer_managed_key_id"}
	instance, err := responseBody.GetSingleOrError()
	if err != nil {
		return nil, err
	}
	if HasMetricsIntegrationEndpointUrl(instance) {
		fields = append(fields, "metrics_integration_url")
	}
	return fields, nil
}

func HasMetricsIntegrationEndpointUrl(instance map[string]any) bool {
	cmiEndpointUrl := instance["metrics_integration_url"]
	switch cmiEndpointUrl := cmiEndpointUrl.(type) {
	case string:
		return len(cmiEndpointUrl) > 0
	}
	return false
}
