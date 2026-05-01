// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/instance"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, `{
			"data": {
				"id": "2f49c2b3",
				"name": "Production",
				"status": "running",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"connection_url": "YOUR_CONNECTION_URL",
				"metrics_integration_url": "YOUR_METRICS_INTEGRATION_ENDPOINT",
				"region": "europe-west1",
				"type": "enterprise-db",
				"memory": "8GB",
				"storage": "16GB"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"metrics_integration_url": "YOUR_METRICS_INTEGRATION_ENDPOINT",
		"name": "Production",
		"region": "europe-west1",
		"status": "running",
		"storage": "16GB",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "enterprise-db"
	  }
	}`)
}

func TestGetEnterpriseInstanceWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, `{
			"data": {
				"id": "2f49c2b3",
				"name": "Production",
				"status": "running",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"connection_url": "YOUR_CONNECTION_URL",
				"metrics_integration_url": "YOUR_METRICS_INTEGRATION_ENDPOINT",
				"region": "europe-west1",
				"type": "enterprise-db",
				"memory": "8GB",
				"storage": "16GB"
			}
		}`)

	// TODO: Make a better way to override config
	helper.SetConfigValue("output", "default")

	helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────┬────────────┬────────────────┬─────────┬─────────────────────┬────────────────┬──────────────┬───────────────┬────────┬─────────┬─────────────────────────┬───────────────────────────────────┐
│ ID       │ NAME       │ TENANT_ID      │ STATUS  │ CONNECTION_URL      │ CLOUD_PROVIDER │ REGION       │ TYPE          │ MEMORY │ STORAGE │ CUSTOMER_MANAGED_KEY_ID │ METRICS_INTEGRATION_URL           │
├──────────┼────────────┼────────────────┼─────────┼─────────────────────┼────────────────┼──────────────┼───────────────┼────────┼─────────┼─────────────────────────┼───────────────────────────────────┤
│ 2f49c2b3 │ Production │ YOUR_TENANT_ID │ running │ YOUR_CONNECTION_URL │ gcp            │ europe-west1 │ enterprise-db │ 8GB    │ 16GB    │                         │ YOUR_METRICS_INTEGRATION_ENDPOINT │
└──────────┴────────────┴────────────────┴─────────┴─────────────────────┴────────────────┴──────────────┴───────────────┴────────┴─────────┴─────────────────────────┴───────────────────────────────────┘
`)

}

func TestGetProfessionalInstanceWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, `{
			"data": {
				"id": "2f49c2b3",
				"name": "Production",
				"status": "running",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"connection_url": "YOUR_CONNECTION_URL",
				"region": "europe-west1",
				"type": "professional-db",
				"memory": "8GB",
				"storage": "16GB"
			}
		}`)

	// TODO: Make a better way to override config
	helper.SetConfigValue("output", "default")

	helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────┬────────────┬────────────────┬─────────┬─────────────────────┬────────────────┬──────────────┬─────────────────┬────────┬─────────┬─────────────────────────┐
│ ID       │ NAME       │ TENANT_ID      │ STATUS  │ CONNECTION_URL      │ CLOUD_PROVIDER │ REGION       │ TYPE            │ MEMORY │ STORAGE │ CUSTOMER_MANAGED_KEY_ID │
├──────────┼────────────┼────────────────┼─────────┼─────────────────────┼────────────────┼──────────────┼─────────────────┼────────┼─────────┼─────────────────────────┤
│ 2f49c2b3 │ Production │ YOUR_TENANT_ID │ running │ YOUR_CONNECTION_URL │ gcp            │ europe-west1 │ professional-db │ 8GB    │ 16GB    │                         │
└──────────┴────────────┴────────────────┴─────────┴─────────────────────┴────────────────┴──────────────┴─────────────────┴────────┴─────────┴─────────────────────────┘
`)

}

func TestGetInstanceNotFoundError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusNotFound, fmt.Sprintf(`{
		"errors": [
			{
			"message": "DB not found: %s",
			"reason": "db-not-found"
			}
		]
	}`, instanceId))

	helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertErr(fmt.Sprintf("Error: [DB not found: %s]", instanceId))
}

func TestGetHasCmiEndpoint(t *testing.T) {
	assert.True(t, instance.HasMetricsIntegrationEndpointUrl(map[string]any{
		"metrics_integration_url": "https://neo4j.io/abc",
	}))
	assert.False(t, instance.HasMetricsIntegrationEndpointUrl(map[string]any{}))
	assert.False(t, instance.HasMetricsIntegrationEndpointUrl(map[string]any{
		"metrics_integration_url": "",
	}))
	assert.False(t, instance.HasMetricsIntegrationEndpointUrl(map[string]any{
		"metrics_integration_url": 1,
	}))
	assert.False(t, instance.HasMetricsIntegrationEndpointUrl(map[string]any{
		"metrics_integration_url": nil,
	}))
}

func TestUnauthorizedAccessTokenRefresh(t *testing.T) {
	statusCodes := []int{http.StatusUnauthorized, http.StatusForbidden}

	for _, statusCode := range statusCodes {
		t.Run(fmt.Sprintf("access token is cleared on status code %d", statusCode), func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			instanceId := "2f49c2b3"

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), statusCode, `{
				"errors": [
					{
						"message": "string",
						"reason": "string",
						"field": "string"
					}
				]
			}`)

			helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodGet)

			helper.AssertCredentialsValue("aura.credentials", `[
	{
		"name": "test-cred",
		"client-id": "",
		"client-secret": "",
		"access-token": "",
		"token-expiry": 0
	}
]`)

			helper.AssertErr(`Error: [
	string,
	Request failed authorization - access token has been cleared and will be refreshed on next request - please retry the command
]`)
		})
	}
}
