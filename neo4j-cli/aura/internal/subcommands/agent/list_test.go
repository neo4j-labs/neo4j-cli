// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListAgents(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusOK, `[
		{
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"enabled": true
		},
		{
			"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"name": "Second Agent",
			"description": "Another agent",
			"dbid": "e5f6g7h8",
			"enabled": false
		}
	]`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent list --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`[
		{
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"enabled": true
		},
		{
			"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"name": "Second Agent",
			"description": "Another agent",
			"dbid": "e5f6g7h8",
			"enabled": false
		}
	]`)
}

func TestListAgentsWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusOK, `[
		{
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"enabled": true
		}
	]`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand("agent list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`[
		{
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"enabled": true
		}
	]`)
}

func TestListAgentsWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusOK, `[
		{
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"name": "My Agent",
			"description": "An agent that queries the database",
			"dbid": "a1b2c3d4",
			"enabled": true
		},
		{
			"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"name": "Second Agent",
			"description": "Another agent",
			"dbid": "e5f6g7h8",
			"enabled": false
		}
	]`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf("agent list --organization-id=%s --project-id=%s", organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────────────────────────────────┬──────────────┬────────────────────────────────────┬──────────┬─────────┐
│ ID                                   │ NAME         │ DESCRIPTION                        │ DBID     │ ENABLED │
├──────────────────────────────────────┼──────────────┼────────────────────────────────────┼──────────┼─────────┤
│ f47ac10b-58cc-4372-a567-0e02b2c3d479 │ My Agent     │ An agent that queries the database │ a1b2c3d4 │ true    │
│ a1b2c3d4-e5f6-7890-abcd-ef1234567890 │ Second Agent │ Another agent                      │ e5f6g7h8 │ false   │
└──────────────────────────────────────┴──────────────┴────────────────────────────────────┴──────────┴─────────┘
	`)
}

func TestListAgentsWithMissingProjectId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf("agent list --organization-id=%s", organizationId))

	helper.AssertErr("Error: required flag(s) \"project-id\" not set")
}

func TestListAgentsWithMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf("agent list --project-id=%s", projectId))

	helper.AssertErr("Error: required flag(s) \"organization-id\" not set")
}
