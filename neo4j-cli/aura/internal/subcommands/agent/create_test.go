// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateAgent(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusCreated, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent create --name "My Agent" --description "An agent that queries the database" --dbid a1b2c3d4 --tools '%s' --organization-id %s --project-id %s --rw`,
		testTools, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true,
		"system_prompt": "",
		"tools": %s
	}`, testTools))

	helper.AssertOutJson(`{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)
}

func TestCreateAgentWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusCreated, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent create --name "My Agent" --description "An agent that queries the database" --dbid a1b2c3d4 --tools '%s' --rw`,
		testTools,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(`{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)
}

func TestCreateAgentWithPrivateFlag(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusCreated, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "Private Agent",
		"description": "A private agent",
		"dbid": "a1b2c3d4",
		"is_private": true,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent create --name "Private Agent" --description "A private agent" --dbid a1b2c3d4 --is-private --tools '%s' --organization-id %s --project-id %s --rw`,
		testTools, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{
		"name": "Private Agent",
		"description": "A private agent",
		"dbid": "a1b2c3d4",
		"is_private": true,
		"is_mcp_enabled": false,
		"enabled": true,
		"system_prompt": "",
		"tools": %s
	}`, testTools))

	helper.AssertOutJson(`{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "Private Agent",
		"description": "A private agent",
		"dbid": "a1b2c3d4",
		"is_private": true,
		"is_mcp_enabled": false,
		"enabled": true
	}`)
}

func TestCreateAgentWithMissingName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent create --description "An agent" --dbid a1b2c3d4 --tools '%s' --organization-id %s --project-id %s --rw`,
		testTools, organizationId, projectId,
	))

	helper.AssertErr("Error: required flag(s) \"name\" not set")
}

func TestCreateAgentWithInvalidToolsJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	helper.SetConfigValue("flag.aura-beta", true)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent create --name "My Agent" --description "An agent" --dbid a1b2c3d4 --tools "not-valid-json" --organization-id %s --project-id %s --rw`,
		organizationId, projectId,
	))

	helper.AssertErr("Error: invalid tools JSON: invalid character 'o' in literal null (expecting 'u')")
}

func TestCreateAgentWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/agents", organizationId, projectId), http.StatusCreated, `{
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"name": "My Agent",
		"description": "An agent that queries the database",
		"dbid": "a1b2c3d4",
		"is_private": false,
		"is_mcp_enabled": false,
		"enabled": true
	}`)

	helper.SetConfigValue("flag.aura-beta", true)
	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent create --name "My Agent" --description "An agent that queries the database" --dbid a1b2c3d4 --tools '%s' --organization-id %s --project-id %s --rw`,
		testTools, organizationId, projectId,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOut(`
┌──────────────────────────────────────┬──────────┬────────────────────────────────────┬──────────┬────────────┬────────────────┬─────────┐
│ ID                                   │ NAME     │ DESCRIPTION                        │ DBID     │ IS_PRIVATE │ IS_MCP_ENABLED │ ENABLED │
├──────────────────────────────────────┼──────────┼────────────────────────────────────┼──────────┼────────────┼────────────────┼─────────┤
│ f47ac10b-58cc-4372-a567-0e02b2c3d479 │ My Agent │ An agent that queries the database │ a1b2c3d4 │ false      │ false          │ true    │
└──────────────────────────────────────┴──────────┴────────────────────────────────────┴──────────┴────────────┴────────────────┴─────────┘
	`)
}
