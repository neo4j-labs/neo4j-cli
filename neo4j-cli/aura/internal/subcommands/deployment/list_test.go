// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusOK, `{
		"data": [{
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"connection_url": "",
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running"
		},
		{
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"connection_url": "http://localhost:7876",
			"id": "11881319-d19d-4337-914b-ed50f238d4be",
			"name": "Test Deployment 2",
			"status": "running"
		}]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment list --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [{
			"connection_url": "",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running"
		},
		{
			"connection_url": "http://localhost:7876",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"id": "11881319-d19d-4337-914b-ed50f238d4be",
			"name": "Test Deployment 2",
			"status": "running"
		}]
	}`)
}

func TestListDeploymentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusOK, `{
		"data": [{
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"connection_url": "",
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running"
			},
			{
				"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"connection_url": "http://localhost:7876",
				"id": "11881319-d19d-4337-914b-ed50f238d4be",
				"name": "Test Deployment 2",
				"status": "running"
				}]
				}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand("deployment list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [{
			"connection_url": "",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running"
		},
		{
			"connection_url": "http://localhost:7876",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"id": "11881319-d19d-4337-914b-ed50f238d4be",
			"name": "Test Deployment 2",
			"status": "running"
		}]
	}`)
}

func TestListDeploymentWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusOK, `{
		"data": [{
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"connection_url": "",
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running"
		},
		{
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"connection_url": "http://localhost:7876",
			"id": "11881319-d19d-4337-914b-ed50f238d4be",
			"name": "Test Deployment 2",
			"status": "running"
		}]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "table")
	helper.ExecuteCommand(fmt.Sprintf("deployment list --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────────────────────────────────┬───────────────────┬──────────────────────────────────────┬─────────┬───────────────────────┐
│ ID                                   │ NAME              │ CREATED_BY                           │ STATUS  │ CONNECTION_URL        │
├──────────────────────────────────────┼───────────────────┼──────────────────────────────────────┼─────────┼───────────────────────┤
│ 9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2 │ Test Deployment   │ 941d32f6-6abf-42d7-beb8-012341376dc6 │ running │                       │
│ 11881319-d19d-4337-914b-ed50f238d4be │ Test Deployment 2 │ 941d32f6-6abf-42d7-beb8-012341376dc6 │ running │ http://localhost:7876 │
└──────────────────────────────────────┴───────────────────┴──────────────────────────────────────┴─────────┴───────────────────────┘
	`)
}

func TestListDeploymentsWithNoDeployments(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment list --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{"data": []}`)
}

func TestListDeploymentsWithMissingProjectId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment list --organization-id=%s", organizationId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: required flag(s) \"project-id\" not set")
}

func TestListDeploymentsWithMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment list --project-id=%s", projectId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: required flag(s) \"organization-id\" not set")
}
