// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetAgent(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent get %s --organization-id=%s --project-id=%s", agentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"is_private": false,
			"is_mcp_enabled": false,
			"enabled": true
		}
	}`)
}

func TestGetAgentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("agent get %s", agentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": {
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"is_private": false,
			"is_mcp_enabled": false,
			"enabled": true
		}
	}`)
}

func TestGetAgentNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "non-existent-agent-id"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s", organizationId, projectId, agentId), http.StatusNotFound, `{
		"errors": [{"message": "Agent not found"}]
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent get %s --organization-id=%s --project-id=%s", agentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertErr(`Error: [
	Agent not found
]`)
}
