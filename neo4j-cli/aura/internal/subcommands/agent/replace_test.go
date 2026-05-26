// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestReplaceAgent(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Updated Agent",
		"description": "An updated description",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent replace %s --name "My Updated Agent" --description "An updated description" --dbid a1b2c3d4 --tools '%s' --organization-id %s --project-id %s --rw`,
		agentId, testTools, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPut)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{
		"name": "My Updated Agent",
		"description": "An updated description",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true,
		"system_prompt": "",
		"tools": %s
	}`, testTools))

	helper.AssertOutJson(`{
	"data": {
		"dbid": "a1b2c3d4",
		"description": "An updated description",
		"enabled": true,
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"is_mcp_enabled": false,
		"is_private": false,
		"name": "My Updated Agent"
	}
}`)
}

func TestReplaceAgentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Updated Agent",
		"description": "An updated description",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent replace %s --name "My Updated Agent" --description "An updated description" --dbid a1b2c3d4 --tools '%s' --rw`,
		agentId, testTools,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPut)

	helper.AssertOutJson(`{
	"data": {
		"dbid": "a1b2c3d4",
		"description": "An updated description",
		"enabled": true,
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"is_mcp_enabled": false,
		"is_private": false,
		"name": "My Updated Agent"
	}
}`)
}

func TestReplaceAgentNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "non-existent-agent-id"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusNotFound, `{
		"errors": [{"message": "Agent not found"}]
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent replace %s --name "My Agent" --description "desc" --dbid a1b2c3d4 --tools '%s' --organization-id %s --project-id %s --rw`,
		agentId, testTools, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPut)

	helper.AssertErr(`Error: [
	Agent not found
]`)
}
