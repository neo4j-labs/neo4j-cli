// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListDeploymentServer(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": [
			{
				"address": "db-list-2:32001",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"health": "Available",
				"id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"jvm_vendor": "Eclipse Adoptium",
				"jvm_version": "21.0.8",
				"last_ping": "2025-11-11T13:56:54.487554Z",
				"license": {
					"state": "VALID",
					"type": "COMMERCIAL"
				},
				"mode_constraint": "NONE",
				"name": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"os_arch": "amd64",
				"os_name": "Linux",
				"os_version": "6.17.5-arch1-1",
				"plugin_version": "1.1.0",
				"plugins": [
					{
						"filename": "neo4j-fleet-management-plugin-1.1.0-v2025.jar",
						"name": "Neo4j - Fleet Management Plugin",
						"version": "1.1.0"
					}
				],
				"state": "Enabled",
				"status": "offline",
				"version": "2025.08.0"
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment server list --deployment-id=%s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"address": "db-list-2:32001",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"health": "Available",
				"id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"jvm_vendor": "Eclipse Adoptium",
				"jvm_version": "21.0.8",
				"last_ping": "2025-11-11T13:56:54.487554Z",
				"license": {
					"state": "VALID",
					"type": "COMMERCIAL"
				},
				"mode_constraint": "NONE",
				"name": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"os_arch": "amd64",
				"os_name": "Linux",
				"os_version": "6.17.5-arch1-1",
				"plugin_version": "1.1.0",
				"plugins": [
					{
						"filename": "neo4j-fleet-management-plugin-1.1.0-v2025.jar",
						"name": "Neo4j - Fleet Management Plugin",
						"version": "1.1.0"
					}
				],
				"state": "Enabled",
				"status": "offline",
				"version": "2025.08.0"
			}
		]
	}`)
}

func TestListDeploymentServerWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": [
			{
				"address": "db-list-2:32001",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"health": "Available",
				"id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"jvm_vendor": "Eclipse Adoptium",
				"jvm_version": "21.0.8",
				"last_ping": "2025-11-11T13:56:54.487554Z",
				"license": {
					"state": "VALID",
					"type": "COMMERCIAL"
				},
				"mode_constraint": "NONE",
				"name": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"os_arch": "amd64",
				"os_name": "Linux",
				"os_version": "6.17.5-arch1-1",
				"plugin_version": "1.1.0",
				"plugins": [
					{
						"filename": "neo4j-fleet-management-plugin-1.1.0-v2025.jar",
						"name": "Neo4j - Fleet Management Plugin",
						"version": "1.1.0"
					}
				],
				"state": "Enabled",
				"status": "offline",
				"version": "2025.08.0"
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment server list --deployment-id=%s", deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"address": "db-list-2:32001",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"health": "Available",
				"id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"jvm_vendor": "Eclipse Adoptium",
				"jvm_version": "21.0.8",
				"last_ping": "2025-11-11T13:56:54.487554Z",
				"license": {
					"state": "VALID",
					"type": "COMMERCIAL"
				},
				"mode_constraint": "NONE",
				"name": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"os_arch": "amd64",
				"os_name": "Linux",
				"os_version": "6.17.5-arch1-1",
				"plugin_version": "1.1.0",
				"plugins": [
					{
						"filename": "neo4j-fleet-management-plugin-1.1.0-v2025.jar",
						"name": "Neo4j - Fleet Management Plugin",
						"version": "1.1.0"
					}
				],
				"state": "Enabled",
				"status": "offline",
				"version": "2025.08.0"
			}
		]
	}`)
}

func TestListDeploymentServerWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": [
			{
				"address": "db-list-2:32001",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"health": "Available",
				"id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"jvm_vendor": "Eclipse Adoptium",
				"jvm_version": "21.0.8",
				"last_ping": "2025-11-11T13:56:54.487554Z",
				"license": {
					"state": "VALID",
					"type": "COMMERCIAL"
				},
				"mode_constraint": "NONE",
				"name": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"os_arch": "amd64",
				"os_name": "Linux",
				"os_version": "6.17.5-arch1-1",
				"plugin_version": "1.1.0",
				"plugins": [
					{
						"filename": "neo4j-fleet-management-plugin-1.1.0-v2025.jar",
						"name": "Neo4j - Fleet Management Plugin",
						"version": "1.1.0"
					}
				],
				"state": "Enabled",
				"status": "offline",
				"version": "2025.08.0"
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "table")
	helper.ExecuteCommand(fmt.Sprintf("deployment server list --deployment-id=%s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌──────────────────────────────────────┬─────────────────┬──────────────────────────────────────┬─────────┬─────────────────────────────┬───────────┬────────────────┐
│ ID                                   │ ADDRESS         │ NAME                                 │ STATUS  │ LAST_PING                   │ VERSION   │ PLUGIN_VERSION │
├──────────────────────────────────────┼─────────────────┼──────────────────────────────────────┼─────────┼─────────────────────────────┼───────────┼────────────────┤
│ 66c6ee3b-de03-4e8a-ba57-066b34730092 │ db-list-2:32001 │ 66c6ee3b-de03-4e8a-ba57-066b34730092 │ offline │ 2025-11-11T13:56:54.487554Z │ 2025.08.0 │ 1.1.0          │
└──────────────────────────────────────┴─────────────────┴──────────────────────────────────────┴─────────┴─────────────────────────────┴───────────┴────────────────┘
	`)
}

func TestListDeploymentServersWithNoData(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment server list --deployment-id=%s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{"data": []}`)
}
