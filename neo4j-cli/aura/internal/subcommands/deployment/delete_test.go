// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteDeployment(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment delete %s --organization-id %s --project-id %s --rw", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErrContainsStrings([]string{fmt.Sprintf("deployment %s deleted", deploymentId)})
	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"deleted": true,
		"id": "%s"
	}
}`, deploymentId))
}

func TestDeleteDeploymentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment delete %s --rw", deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErrContainsStrings([]string{fmt.Sprintf("deployment %s deleted", deploymentId)})
	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"deleted": true,
		"id": "%s"
	}
}`, deploymentId))
}

func TestDeleteDeploymentWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment delete --organization-id %s --project-id %s --rw %s\"\n\"", organizationId, projectId, deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErrContainsStrings([]string{fmt.Sprintf("deployment %s deleted", deploymentId)})
	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"deleted": true,
		"id": "%s"
	}
}`, deploymentId))
}

// TestDeleteDeployment_StdoutIsValidJSON is the CLI-82 regression-pin
// for the deployment delete narration: pre-fix, stdout had
// "Deployment deleted successfully ..." instead of structured JSON.
func TestDeleteDeployment_StdoutIsValidJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf("deployment delete %s --organization-id %s --project-id %s --rw --format json", deploymentId, organizationId, projectId))

	helper.AssertOutIsValidJSON()
}

func TestDeleteDeploymentWhenDeploymentDoesNotExist(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "87703862-f8b7-4712-b7eb-d0eef69cb53"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s", organizationId, projectId, deploymentId), http.StatusForbidden, `{
		"error": "Access denied"
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment delete %s --organization-id=%s --project-id=%s --rw", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErr("Error: Access denied")
}
