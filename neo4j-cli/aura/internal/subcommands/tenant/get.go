// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tenant

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Hidden: true,
		Use:    "get <id>",
		Short:  "Returns tenant details",
		Long:   "This subcommand returns details about a specific Aura Tenant.",
		Example: `# Get details of a tenant by ID
neo4j-cli aura tenant get 00000000-0000-0000-0000-000000000000

# Get tenant details and emit JSON for scripting
neo4j-cli aura tenant get 00000000-0000-0000-0000-000000000000 --format json

# Pipe details through jq to extract the tenant name
neo4j-cli aura tenant get 00000000-0000-0000-0000-000000000000 --format json | jq -r '.data.name'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: 'aura tenant get' is deprecated and will be removed in a future release. Use 'aura organization get' instead.") //nolint:errcheck // deprecation warning to stderr; write errors are not actionable
			tenantId := strings.TrimSpace(args[0])
			path := fmt.Sprintf("/tenants/%s", tenantId)

			cmd.SilenceUsage = true
			resBody, statusCode, err := api.MakeRequest(cfg, path, &api.RequestConfig{
				Method: http.MethodGet,
			})
			if err != nil {
				return err
			}

			if statusCode == http.StatusOK {
				responseData := api.ParseBody(resBody)
				fields, values, err := postProcessResponseValues(cfg, tenantId, responseData)
				if err != nil {
					return err
				}
				output.PrintBodyMap(cmd, cfg, values, fields)
				if cfg.Global.Format() == "table" || cfg.Global.Format() == "default" {
					fmt.Fprintln(cmd.ErrOrStderr(), "instance configurations are not visible with table output - please use a different output setting using --format if you would like to view these") //nolint:errcheck // narration to stderr; write errors are not actionable
				}
			}

			return nil
		},
	}
}

func postProcessResponseValues(cfg *clicfg.Config, tenantId string, responseData api.ResponseData) ([]string, api.ResponseData, error) {
	metricsIntegrationEndpointUrl, err := getMetricsIntegrationEndpointUrl(cfg, tenantId)
	if err != nil {
		return nil, nil, err
	}
	fields := []string{"id", "name"}
	if len(metricsIntegrationEndpointUrl) > 0 {
		tenant, err := responseData.GetSingleOrError()
		if err != nil {
			return nil, nil, err
		}
		tenant["metrics_integration_url"] = metricsIntegrationEndpointUrl
		return append(fields, "metrics_integration_url"), api.NewSingleValueResponseData(tenant), nil
	} else {
		return fields, responseData, nil
	}
}

func getMetricsIntegrationEndpointUrl(cfg *clicfg.Config, tenantId string) (string, error) {
	resBody, statusCode, err := api.MakeRequest(cfg, fmt.Sprintf("/tenants/%s/metrics-integration", tenantId), &api.RequestConfig{
		Method: http.MethodGet,
	})
	// Aura API (in fact Console API returns HTTP 400 when CMI endpoint is not available for the tenant)
	if err != nil && statusCode != http.StatusBadRequest {
		return "", err
	}
	switch statusCode {
	case http.StatusOK:
		metricsIntegrationResponse := api.ParseBody(resBody)
		metricsIntegration, err := metricsIntegrationResponse.GetSingleOrError()
		if err != nil {
			return "", err
		}
		if endpointUrl, ok := metricsIntegration["endpoint"].(string); ok {
			if len(endpointUrl) > 0 {
				return endpointUrl, nil
			}
		}
		return "", nil
	case http.StatusBadRequest:
		return "", nil
	default:
		panic(clierr.NewFatalError("unexpected statusCode %d", statusCode))
	}
}
