// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tenant_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetTenantWithoutIntegrationEndpoint(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	tenantId := "6981ace7-efe8-4f5c-b7c5-267b5162ce91"

	getMockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s", tenantId), http.StatusOK, `{
			"data": {
				"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
				"name": "Production",
				"instance_configurations": []
			}
		}`)

	metricsIntegrationMockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s/metrics-integration", tenantId), http.StatusBadRequest, `{
			"errors": [
				{
					"message": "This tenant has no instances eligible for metrics integration",
					"reason": "tenant-incapable-of-action"
				}
			]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("tenant get %s", tenantId))

	getMockHandler.AssertCalledTimes(1)
	getMockHandler.AssertCalledWithMethod(http.MethodGet)
	metricsIntegrationMockHandler.AssertCalledTimes(1)
	metricsIntegrationMockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
			"instance_configurations": [],
			"name": "Production"
		}
	}
	`)
}

func TestGetTenantWithMetricsIntegrationEndpoint(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	tenantId := "6981ace7-efe8-4f5c-b7c5-267b5162ce91"

	getMockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s", tenantId), http.StatusOK, `{
			"data": {
				"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
				"name": "Production",
				"instance_configurations": []
			}
		}`)

	metricsIntegrationMockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s/metrics-integration", tenantId), http.StatusOK, `{
			"data": {
				"endpoint": "https://customer-metrics-api-devnommrr.neo4j-dev.io/api/v1/ca7bc96c-204c-546e-9736-f4a578d53f64/metrics"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("tenant get %s", tenantId))

	getMockHandler.AssertCalledTimes(1)
	getMockHandler.AssertCalledWithMethod(http.MethodGet)
	metricsIntegrationMockHandler.AssertCalledTimes(1)
	metricsIntegrationMockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
			"instance_configurations": [],
			"metrics_integration_url": "https://customer-metrics-api-devnommrr.neo4j-dev.io/api/v1/ca7bc96c-204c-546e-9736-f4a578d53f64/metrics",
			"name": "Production"
		}
	}
	`)
}

func TestGetTenantNotFoundError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	tenantId := "6981ace7-efe8-4f5c-b7c5-267b5162ce91"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s", tenantId), http.StatusNotFound, `{
		"errors": [
			{
			"message": "The tenant you specified could not be found",
			"reason": "tenant-not-found"
			}
		]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("tenant get %s", tenantId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertErr("Error: [The tenant you specified could not be found]")
}

func TestGetTenantWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	tenantId := "6981ace7-efe8-4f5c-b7c5-267b5162ce91"

	getMockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s", tenantId), http.StatusOK, `{
			"data": {
				"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
				"name": "Production",
				"instance_configurations": []
			}
		}`)

	metricsIntegrationMockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/tenants/%s/metrics-integration", tenantId), http.StatusBadRequest, `{
			"errors": [
				{
					"message": "This tenant has no instances eligible for metrics integration",
					"reason": "tenant-incapable-of-action"
				}
			]
		}`)

	helper.ExecuteCommand(fmt.Sprintf("tenant get %s --format table", tenantId))

	getMockHandler.AssertCalledTimes(1)
	getMockHandler.AssertCalledWithMethod(http.MethodGet)
	metricsIntegrationMockHandler.AssertCalledTimes(1)
	metricsIntegrationMockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────────────────────────────────┬────────────┐
│ ID                                   │ NAME       │
├──────────────────────────────────────┼────────────┤
│ 6981ace7-efe8-4f5c-b7c5-267b5162ce91 │ Production │
└──────────────────────────────────────┴────────────┘
instance configurations are not visible with table output - please use a different output setting using --format if you would like to view these
`)
}
