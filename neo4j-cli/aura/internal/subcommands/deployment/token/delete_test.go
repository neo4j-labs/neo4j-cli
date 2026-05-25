// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/confirm"
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

func TestDeleteDeploymentTokenConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	err := helper.ExecuteCommandE(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw", deploymentId, organizationId, projectId))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	mockHandler.AssertCalledTimes(0)
}

func TestDeleteDeploymentTokenConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw --yes --force", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteDeploymentTokenConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("y\n")

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertErrContainsStrings([]string{"Delete token"})
}

func TestDeleteDeploymentTokenConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("N\n")

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/token", organizationId, projectId, deploymentId), http.StatusNoContent, "")

	helper.SetConfigValue("flag.aura-beta", true)
	err := helper.ExecuteCommandE(fmt.Sprintf("deployment token delete --deployment-id %s --organization-id %s --project-id %s --rw", deploymentId, organizationId, projectId))

	if err != nil {
		t.Fatalf("expected nil on cancel, got %v", err)
	}
	mockHandler.AssertCalledTimes(0)
	helper.AssertErrContainsStrings([]string{"cancelled."})
}
