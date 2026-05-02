// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"dbms": {
				"edition": "enterprise",
				"metric_collection_enabled": true,
				"packaging": "PACKAGING"
			},
			"token": {
				"id": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"claimed_time": "CLAIMED_BY",
				"expiry_time": "EXPIRY_TIME",
				"last_used_time": "LAST_USED",
				"release_time": "RELEASE_TIME",
				"auto_rotated": true,
				"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"creation_time": "CREATION_TIME"
			}
		}
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"dbms": {
				"edition": "enterprise",
				"metric_collection_enabled": true,
				"packaging": "PACKAGING"
			},
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running",
			"token": {
				"auto_rotated": true,
				"claimed_time": "CLAIMED_BY",
				"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"creation_time": "CREATION_TIME",
				"expiry_time": "EXPIRY_TIME",
				"id": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"last_used_time": "LAST_USED",
				"release_time": "RELEASE_TIME"
			}
		}
	}`)
}

func TestGetDeploymentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"dbms": {
				"edition": "enterprise",
				"metric_collection_enabled": true,
				"packaging": "PACKAGING"
			},
			"token": {
				"id": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"claimed_time": "CLAIMED_BY",
				"expiry_time": "EXPIRY_TIME",
				"last_used_time": "LAST_USED",
				"release_time": "RELEASE_TIME",
				"auto_rotated": true,
				"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"creation_time": "CREATION_TIME"
			}
		}
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s", deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"dbms": {
				"edition": "enterprise",
				"metric_collection_enabled": true,
				"packaging": "PACKAGING"
			},
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running",
			"token": {
				"auto_rotated": true,
				"claimed_time": "CLAIMED_BY",
				"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"creation_time": "CREATION_TIME",
				"expiry_time": "EXPIRY_TIME",
				"id": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"last_used_time": "LAST_USED",
				"release_time": "RELEASE_TIME"
			}
		}
	}`)
}

func TestGetDeploymentWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
			"name": "Test Deployment",
			"status": "running",
			"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
			"dbms": {
				"edition": "enterprise",
				"metric_collection_enabled": true,
				"packaging": "PACKAGING"
			},
			"token": {
				"id": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"claimed_time": "CLAIMED_BY",
				"expiry_time": "EXPIRY_TIME",
				"last_used_time": "LAST_USED",
				"release_time": "RELEASE_TIME",
				"auto_rotated": true,
				"created_by": "941d32f6-6abf-42d7-beb8-012341376dc6",
				"creation_time": "CREATION_TIME"
			}
		}
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "default")
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────────────────────────────────┬─────────────────┬──────────────┬────────────────┬───────────────────┬────────────────────┬─────────────────────┐
│ ID                                   │ NAME            │ DBMS:EDITION │ DBMS:PACKAGING │ TOKEN:EXPIRY_TIME │ TOKEN:AUTO_ROTATED │ TOKEN:CREATION_TIME │
├──────────────────────────────────────┼─────────────────┼──────────────┼────────────────┼───────────────────┼────────────────────┼─────────────────────┤
│ 9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2 │ Test Deployment │ enterprise   │ PACKAGING      │ EXPIRY_TIME       │ true               │ CREATION_TIME       │
└──────────────────────────────────────┴─────────────────┴──────────────┴────────────────┴───────────────────┴────────────────────┴─────────────────────┘
	`)
}

func TestGetDeploymentWithMissingProjectId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "87703862-f8b7-4712-b7eb-d0eef69cb530"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s --organization-id=%s", deploymentId, organizationId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: required flag(s) \"project-id\" not set")
}

func TestGetDeploymentWithMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "87703862-f8b7-4712-b7eb-d0eef69cb530"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s --project-id=%s", deploymentId, projectId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: required flag(s) \"organization-id\" not set")
}

func TestGetDeploymentWithMissingArgs(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "87703862-f8b7-4712-b7eb-d0eef69cb530"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment get --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: accepts 1 arg(s), received 0")
}

func TestGetDeploymentWithInvalidDeploymentId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "87703862-f8b7-4712-b7eb-d0eef69cb53"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusBadRequest, `{
		"errors": [{"message": "cannot parse UUID  87703862-f8b7-4712-b7eb-d0eef69cb53"}]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertErr("Error: [cannot parse UUID  87703862-f8b7-4712-b7eb-d0eef69cb53]")
}

func TestGetDeploymentWhenDeploymentDoesNotExist(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "87703862-f8b7-4712-b7eb-d0eef69cb53"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusForbidden, `{
		"error": "Access denied"
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment get %s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertErr("Error: Access denied")
}
