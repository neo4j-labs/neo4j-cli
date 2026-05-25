// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteAgent(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusAccepted, "")

	helper.ExecuteCommand(fmt.Sprintf("agent delete %s --organization-id=%s --project-id=%s --rw --yes --force", agentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("Agent deleted successfully f47ac10b-58cc-4372-a567-0e02b2c3d479")
}

func TestDeleteAgentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusAccepted, "")

	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("agent delete %s --rw --yes --force", agentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOut("Agent deleted successfully f47ac10b-58cc-4372-a567-0e02b2c3d479")
}

func TestDeleteAgentNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "non-existent-agent-id"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusNotFound, `{
		"errors": [{"message": "Agent not found"}]
	}`)

	helper.ExecuteCommand(fmt.Sprintf("agent delete %s --organization-id=%s --project-id=%s --rw --yes --force", agentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertErr(`Error: [
	Agent not found
]`)
}

func TestDeleteAgentConfirmGate(t *testing.T) {
	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	base := fmt.Sprintf("agent delete %s --organization-id=%s --project-id=%s --rw", agentId, organizationId, projectId)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura agent delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "agent",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			mock := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusAccepted, "")
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
