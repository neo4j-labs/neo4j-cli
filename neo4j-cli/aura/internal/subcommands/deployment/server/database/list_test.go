// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package serverdatabase_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListDeploymentServerDatabase(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	serverId := "66c6ee3b-de03-4e8a-ba57-066b34730092"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers/%s/databases", organizationId, projectId, deploymentId, serverId), http.StatusOK, `{
		"data": [
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_committed_txn": 2,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "neo4j",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "standard",
				"writer": true
			},
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_committed_txn": 139,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "system",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "system",
				"writer": true
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment server database list --deployment-id=%s --server-id=%s --organization-id=%s --project-id=%s", deploymentId, serverId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_committed_txn": 2,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "neo4j",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "standard",
				"writer": true
			},
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_committed_txn": 139,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "system",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "system",
				"writer": true
			}
		]
	}`)
}

func TestListDeploymentServerDatabaseWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	serverId := "66c6ee3b-de03-4e8a-ba57-066b34730092"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers/%s/databases", organizationId, projectId, deploymentId, serverId), http.StatusOK, `{
		"data": [
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_committed_txn": 2,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "neo4j",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "standard",
				"writer": true
			},
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_committed_txn": 139,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "system",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "system",
				"writer": true
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment server database list --deployment-id=%s --server-id=%s", deploymentId, serverId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_committed_txn": 2,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "neo4j",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "standard",
				"writer": true
			},
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_committed_txn": 139,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "system",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "system",
				"writer": true
			}
		]
	}`)
}

func TestListDeploymentServerDatabaseWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	serverId := "66c6ee3b-de03-4e8a-ba57-066b34730092"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers/%s/databases", organizationId, projectId, deploymentId, serverId), http.StatusOK, `{
		"data": [
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_committed_txn": 2,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "neo4j",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "standard",
				"writer": true
			},
			{
				"current_status": "online",
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"graph_shards": null,
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_committed_txn": 139,
				"last_seen": "2025-11-11T13:56:54.487554Z",
				"name": "system",
				"property_shards": null,
				"replication_lag": 0,
				"role": "primary",
				"server_id": "66c6ee3b-de03-4e8a-ba57-066b34730092",
				"status_message": "",
				"type": "system",
				"writer": true
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "table")
	helper.ExecuteCommand(fmt.Sprintf("deployment server database list --deployment-id=%s --server-id=%s --organization-id=%s --project-id=%s", deploymentId, serverId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌────────┬──────────┬────────────────┬────────────────────┬─────────────────────────────┬─────────────────┬─────────┬────────┐
│ NAME   │ TYPE     │ CURRENT_STATUS │ LAST_COMMITTED_TXN │ LAST_SEEN                   │ REPLICATION_LAG │ ROLE    │ WRITER │
├────────┼──────────┼────────────────┼────────────────────┼─────────────────────────────┼─────────────────┼─────────┼────────┤
│ neo4j  │ standard │ online         │ 2                  │ 2025-11-11T13:56:54.487554Z │ 0               │ primary │ true   │
│ system │ system   │ online         │ 139                │ 2025-11-11T13:56:54.487554Z │ 0               │ primary │ true   │
└────────┴──────────┴────────────────┴────────────────────┴─────────────────────────────┴─────────────────┴─────────┴────────┘
	`)
}

func TestListDeploymentServerDatabasesWithNoData(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"
	serverId := "66c6ee3b-de03-4e8a-ba57-066b34730092"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/servers/%s/databases", organizationId, projectId, deploymentId, serverId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment server database list --deployment-id=%s --server-id=%s --organization-id=%s --project-id=%s", deploymentId, serverId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{"data": []}`)
}
