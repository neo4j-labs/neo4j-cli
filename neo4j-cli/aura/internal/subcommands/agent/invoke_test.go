// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
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

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{"input": "What movies are in the database?"}`)

	helper.AssertOutJson(`{
	"data": {
		"content": [
			{
				"text": "Here are the movies in the database...",
				"type": "text"
			}
		],
		"end_reason": "end_turn",
		"id": "inv-12345",
		"role": "assistant",
		"status": "completed",
		"type": "message",
		"usage": {
			"request_tokens": 150,
			"response_tokens": 200,
			"total_tokens": 350
		}
	}
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

	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --rw`,
		agentId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(`{
	"data": {
		"content": [
			{
				"text": "Here are the movies in the database...",
				"type": "text"
			}
		],
		"end_reason": "end_turn",
		"id": "inv-12345",
		"role": "assistant",
		"status": "completed",
		"type": "message",
		"usage": {
			"request_tokens": 150,
			"response_tokens": 200,
			"total_tokens": 350
		}
	}
}`)
}

func TestInvokeAgentWithMissingInput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

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
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-403")

	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErr("Error: agent invocation forbidden: agent may be disabled or private (invocation id: inv-abc-403)")
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
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-err")

	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErr("Error: agent invocation failed: model context length exceeded (invocation id: inv-abc-err)")
}

// TestInvokeAgentToolFailureSuccessPath pins the HTTP-200 status:"completed"
// case where a *_tool_result block reports a tool failure in content[]: the
// command still succeeds and the invocation id is appended to the stats line.
func TestInvokeAgentToolFailureSuccessPath(t *testing.T) {
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
			{"type": "cypher_template_tool_result", "is_error": true, "error": {"message": "query failed"}},
			{"type": "text", "text": "I could not complete the request."}
		],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-tool")

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertOut("I could not complete the request.\n\nStatus: COMPLETED | End reason: END TURN | Tool calls: 1 | Tokens: 150 req / 200 res / 350 total | Invocation ID: inv-abc-tool")
}

func TestInvokeAgentJsonSuccessPrintsInvocationIdToStderr(t *testing.T) {
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
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-json")

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertOutIsValidJSON()
	helper.AssertErr("Invocation ID: inv-abc-json")
}

// TestInvokeAgentUpstreamErrorIncludesInvocationId pins that a 500 upstream
// error (routed through clierr as a *CLIError, exit 8) still carries the
// invocation id. Asserts both the cobra-printed stderr summary AND — via the
// returned error — that errors.As still recovers a *CLIError whose Message
// carries the id and whose exit Code stays 8 (the contract clierr.Render and
// the --format json envelope render from). The test harness does not invoke
// clierr.Render (that lives in main.go), so the envelope shape is verified
// against the structured error directly.
func TestInvokeAgentUpstreamErrorIncludesInvocationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusInternalServerError, `{
		"errors": [{"message": "Internal server error"}]
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-500")

	err := helper.ExecuteCommandE(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErrContainsStrings([]string{"Internal server error", "(invocation id: inv-abc-500)"})

	var ce *clierr.CLIError
	assert.True(t, errors.As(err, &ce), "expected a *clierr.CLIError")
	assert.Contains(t, ce.Message, "(invocation id: inv-abc-500)")
	assert.Contains(t, ce.Message, "Internal server error")
	env := ce.BuildEnvelope()
	assert.Equal(t, 8, env.Error.ExitCode)
	assert.Equal(t, "upstream_error", env.Error.Code)
	assert.Contains(t, env.Error.Message, "(invocation id: inv-abc-500)")
}

// TestInvokeAgentNotFoundIncludesInvocationId pins that a 404 (routed through
// clierr as NewNotFoundError, exit 3) carries the invocation id and preserves
// exit 3.
func TestInvokeAgentNotFoundIncludesInvocationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "non-existent-agent-id"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusNotFound, `{
		"errors": [{"message": "Agent not found"}]
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-404")

	err := helper.ExecuteCommandE(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErrContainsStrings([]string{"Agent not found", "(invocation id: inv-abc-404)"})

	var ce *clierr.CLIError
	assert.True(t, errors.As(err, &ce), "expected a *clierr.CLIError")
	assert.Equal(t, 3, ce.Code)
	assert.Contains(t, ce.Message, "(invocation id: inv-abc-404)")
}

// TestInvokeAgentAuthErrorIncludesInvocationId pins that a 401 (routed through
// clierr as NewAuthError, exit 4) carries the invocation id and preserves exit
// 4. The 403 branch in invoke.go substitutes its own plain forbidden message,
// so 401 is the representative server-auth CLIError path through the generic
// return.
func TestInvokeAgentAuthErrorIncludesInvocationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusUnauthorized, `{
		"errors": [{"message": "Not authenticated"}]
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-401")

	err := helper.ExecuteCommandE(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErrContainsStrings([]string{"Not authenticated", "(invocation id: inv-abc-401)"})

	var ce *clierr.CLIError
	assert.True(t, errors.As(err, &ce), "expected a *clierr.CLIError")
	assert.Equal(t, 4, ce.Code)
	assert.Contains(t, ce.Message, "(invocation id: inv-abc-401)")
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

	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "hello" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertErr(`Error: [
	Agent not found
]`)
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
	}`).WithResponseHeader("X-Agent-Invocation-Id", "inv-abc-table")

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOut("Here are the movies in the database.\n\nStatus: COMPLETED | End reason: END TURN | Tool calls: 1 | Tokens: 150 req / 200 res / 350 total | Invocation ID: inv-abc-table")
}

// TestInvokeAgentNoInvocationIdHeader pins that when the header is absent the
// stats line carries no trailing " | Invocation ID:" and stderr is clean.
func TestInvokeAgentNoInvocationIdHeader(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	agentId := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents/%s/invoke", organizationId, projectId, agentId), http.StatusOK, `{
		"id": "inv-12345",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Here are the movies in the database."}],
		"end_reason": "end_turn",
		"status": "completed",
		"usage": {"request_tokens": 150, "response_tokens": 200, "total_tokens": 350}
	}`)

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent invoke %s --input "What movies are in the database?" --organization-id %s --project-id %s --rw`,
		agentId, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)

	helper.AssertOut("Here are the movies in the database.\n\nStatus: COMPLETED | End reason: END TURN | Tool calls: 0 | Tokens: 150 req / 200 res / 350 total")
	helper.AssertErr("")
}
