// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteDeploymentToken(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw --yes --force", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErrContainsStrings([]string{fmt.Sprintf("deployment-token for deployment %s deleted", deploymentId)})
	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"deleted": true,
		"deployment_id": "%s"
	}
}`, deploymentId))
}

func TestDeleteDeploymentTokenWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --rw --yes --force", deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErrContainsStrings([]string{fmt.Sprintf("deployment-token for deployment %s deleted", deploymentId)})
	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"deleted": true,
		"deployment_id": "%s"
	}
}`, deploymentId))
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

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw --yes --force", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErr("Error: Access denied")
}

func TestDeleteDeploymentTokenConfirmGate(t *testing.T) {
	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	base := fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw", deploymentId, organizationId, projectId)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura deployment token delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "token",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			helper.SetConfigValue("flag.aura-beta", true)
			mock := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
