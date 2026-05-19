// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestInvokeAgent(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "inv-12345",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Here are the movies in the database..."}],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"input": "What movies are in the database?"}`)

	helper.AssertOutJson(`{
		"id": "inv-12345",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Here are the movies in the database..."}],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`)
}

func TestInvokeAgentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "inv-12345",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Here are the movies in the database..."}],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --rw`,
		agentId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(`{
		"id": "inv-12345",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Here are the movies in the database..."}],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`)
}

func TestInvokeAgentWithMissingInput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf("agent invoke %s --organization-id %s --project-id %s --rw", agentId, organizationId, projectId))

	helper.AssertErr("Error: required flag(s) \"input\" not set")
}

func TestInvokeAgentForbidden(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusForbidden, `{
		"error": "agent is private"
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErr("Error: agent invocation forbidden: agent may be disabled or private")
}

func TestInvokeAgentApplicationError(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "inv-99999",
		"type": "error",
		"status": "failed",
		"error": {"message": "model context length exceeded", "type": "context_length_error", "status_code": 400}
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErr("Error: agent invocation failed: model context length exceeded")
}

func TestInvokeAgentNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "non-existent-agent-id"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusNotFound, `{
		"errors": [{"message": "Agent not found"}]
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErr("Error: [Agent not found]")
}

func TestInvokeAgentWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "inv-12345",
		"type": "message",
		"role": "assistant",
		"content": [
			{"type": "cypher_template_tool_use", "id": "tool-1"},
			{"type": "cypher_template_tool_result", "output": {}},
			{"type": "text", "text": "Here are the movies in the database."}
		],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOut("Here are the movies in the database.\n\nStatus: COMPLETED | End reason: END TURN | Tool calls: 1 | Tokens: 150 req / 200 res / 350 total")
}
