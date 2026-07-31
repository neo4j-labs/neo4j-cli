// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/instance"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

// scopedInstancePath builds the v2beta1 org/project-scoped instance path used
// by the migrated get/delete commands.
func scopedInstancePath(instanceID string) string {
	return "/v2beta1/organizations/" + testListOrgID + "/projects/" + testListProjectID + "/instances/" + instanceID
}

func TestGetInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusOK, `{
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

	helper.ExecuteCommand(fmt.Sprintf("instance get %s --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

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
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "running",
		"storage": "16GB",
		"type": "enterprise-db"
	  }
	}`)
}

func TestGetInstanceWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusOK, `{
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

	helper.ExecuteCommand(fmt.Sprintf("instance get %s\"\n\" --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

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
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "running",
		"storage": "16GB",
		"type": "enterprise-db"
	  }
	}`)
}

func TestGetEnterpriseInstanceWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusOK, `{
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

	helper.SetConfigValue("format", "table")

	helper.ExecuteCommand(fmt.Sprintf("instance get %s --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────┬────────────┬────────────────┬─────────┬─────────────────────┬────────────────┬──────────────┬───────────────┬────────┬─────────┬─────────────────────────┬───────────────────────────────────┐
│ ID       │ NAME       │ PROJECT_ID     │ STATUS  │ CONNECTION_URL      │ CLOUD_PROVIDER │ REGION       │ TYPE          │ MEMORY │ STORAGE │ CUSTOMER_MANAGED_KEY_ID │ METRICS_INTEGRATION_URL           │
├──────────┼────────────┼────────────────┼─────────┼─────────────────────┼────────────────┼──────────────┼───────────────┼────────┼─────────┼─────────────────────────┼───────────────────────────────────┤
│ 2f49c2b3 │ Production │ YOUR_TENANT_ID │ running │ YOUR_CONNECTION_URL │ gcp            │ europe-west1 │ enterprise-db │ 8GB    │ 16GB    │                         │ YOUR_METRICS_INTEGRATION_ENDPOINT │
└──────────┴────────────┴────────────────┴─────────┴─────────────────────┴────────────────┴──────────────┴───────────────┴────────┴─────────┴─────────────────────────┴───────────────────────────────────┘
`)

}

func TestGetProfessionalInstanceWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusOK, `{
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

	helper.SetConfigValue("format", "table")

	helper.ExecuteCommand(fmt.Sprintf("instance get %s --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────┬────────────┬────────────────┬─────────┬─────────────────────┬────────────────┬──────────────┬─────────────────┬────────┬─────────┬─────────────────────────┐
│ ID       │ NAME       │ PROJECT_ID     │ STATUS  │ CONNECTION_URL      │ CLOUD_PROVIDER │ REGION       │ TYPE            │ MEMORY │ STORAGE │ CUSTOMER_MANAGED_KEY_ID │
├──────────┼────────────┼────────────────┼─────────┼─────────────────────┼────────────────┼──────────────┼─────────────────┼────────┼─────────┼─────────────────────────┤
│ 2f49c2b3 │ Production │ YOUR_TENANT_ID │ running │ YOUR_CONNECTION_URL │ gcp            │ europe-west1 │ professional-db │ 8GB    │ 16GB    │                         │
└──────────┴────────────┴────────────────┴─────────┴─────────────────────┴────────────────┴──────────────┴─────────────────┴────────┴─────────┴─────────────────────────┘
`)

}

func TestGetInstanceWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testListOrgID, testListProjectID)

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusOK, `{
			"data": {
				"id": "2f49c2b3",
				"name": "Production",
				"status": "running",
				"tenant_id": "YOUR_TENANT_ID",
				"cloud_provider": "gcp",
				"connection_url": "YOUR_CONNECTION_URL",
				"region": "europe-west1",
				"type": "enterprise-db",
				"memory": "8GB",
				"storage": "16GB"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AsssertOk()
}

func TestGetInstanceMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	helper.ExecuteCommand(fmt.Sprintf("instance get %s", instanceId))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestGetInstanceMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	helper.ExecuteCommand(fmt.Sprintf("instance get %s --organization-id %s", instanceId, testListOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestGetInstanceProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	instanceId := "2f49c2b3"
	helper.ExecuteCommand(fmt.Sprintf("instance get %s --organization-id %s --project-id unknown-project", instanceId, testListOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testListOrgID)
}

// TestGetInstanceNotInProject covers an instance that does not belong to the
// scoped project. With the v2beta1 scoped path there is no tenant_id preflight;
// the API returns a native 404 that carries the correct resource envelope.
func TestGetInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusNotFound, fmt.Sprintf(`{
		"errors": [
			{
			"message": "DB not found: %s",
			"reason": "db-not-found"
			}
		]
	}`, instanceId))

	err := helper.ExecuteCommandE(fmt.Sprintf("instance get %s --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)
	require.Equal(t, "instance", ce.ResourceType)
	require.Equal(t, instanceId, ce.ResourceID)

	helper.AssertErr(fmt.Sprintf("Error: [\n\tDB not found: %s\n]", instanceId))
}

func TestGetInstanceNotFoundError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusNotFound, fmt.Sprintf(`{
		"errors": [
			{
			"message": "DB not found: %s",
			"reason": "db-not-found"
			}
		]
	}`, instanceId))

	err := helper.ExecuteCommandE(fmt.Sprintf("instance get %s --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)

	helper.AssertErr(fmt.Sprintf("Error: [\n\tDB not found: %s\n]", instanceId))
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

			registerProjectsMock(&helper)

			instanceId := "2f49c2b3"

			mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), statusCode, `{
				"errors": [
					{
						"message": "string",
						"reason": "string",
						"field": "string"
					}
				]
			}`)

			helper.ExecuteCommand(fmt.Sprintf("instance get %s --organization-id %s --project-id %s", instanceId, testListOrgID, testListProjectID))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodGet)

			helper.AssertCredentialsValue("aura.credentials", `[
	{
		"name": "test-cred",
		"client-id": "test-client-id",
		"client-secret": "test-client-secret",
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
