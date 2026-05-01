// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	name := "Test Deployment"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusCreated, `{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
		}
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment create --name \"%s\" --organization-id %s --project-id %s", name, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{"connection_url":"","name":"%s"}`, name))

	helper.AssertOutJson(`{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
		}
	}`)
}

func TestCreateDeploymentWithOrganizationAndProjectIdFromDefaultProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	name := "Test Deployment"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusCreated, `{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
		}
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment create --name \"%s\"", name))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{"connection_url":"","name":"%s"}`, name))

	helper.AssertOutJson(`{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
		}
	}`)
}

func TestCreateDeploymentWithConnectionUrl(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	name := "Test Deployment"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusCreated, `{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
		}
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment create --name \"%s\" --connection-url \"http://localhost:7876\" --organization-id %s --project-id %s", name, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{"connection_url":"http://localhost:7876","name":"%s"}`, name))

	helper.AssertOutJson(`{
		"data": {
			"id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
		}
	}`)
}

func TestCreateDeploymentWithMissingName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusCreated, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment create --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr("Error: required flag(s) \"name\" not set")
}

func TestCreateDeploymentWithTooLongName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	name := "A very long name that is not accepted by the API"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments", organizationId, projectId), http.StatusBadRequest, `{
		"errors": [{"message": "Name must be between 1 and 30 characters"}]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment create --name=\"%s\" --organization-id=%s --project-id=%s", name, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertErr("Error: [Name must be between 1 and 30 characters]")
}
