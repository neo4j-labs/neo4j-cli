// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteDeploymentToken(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("Deployment token deleted successfully for deployment 9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2")
}

func TestDeleteDeploymentTokenWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s", deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("Deployment token deleted successfully for deployment 9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2")
}

func TestDeleteDeploymentTokenWhenDeploymentDoesNotExist(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusForbidden, `{
		"error": "Access denied"
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErr("Error: Access denied")
}
