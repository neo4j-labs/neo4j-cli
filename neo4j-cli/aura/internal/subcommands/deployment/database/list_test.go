// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListDeploymentDatabase(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/databases", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": [
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": true,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "neo4j",
				"node_count": 0,
				"relationship_count": 0,
				"requested_primaries_count": 1,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "block-block-1.1"
			},
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": false,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "system",
				"node_count": null,
				"relationship_count": null,
				"requested_primaries_count": 0,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "record-aligned-1.1"
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment database list --deployment-id=%s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": true,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "neo4j",
				"node_count": 0,
				"relationship_count": 0,
				"requested_primaries_count": 1,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "block-block-1.1"
			},
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": false,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "system",
				"node_count": null,
				"relationship_count": null,
				"requested_primaries_count": 0,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "record-aligned-1.1"
			}
		]
	}`)
}

func TestListDeploymentDatabaseWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/databases", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": [
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": true,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "neo4j",
				"node_count": 0,
				"relationship_count": 0,
				"requested_primaries_count": 1,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "block-block-1.1"
			},
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": false,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "system",
				"node_count": null,
				"relationship_count": null,
				"requested_primaries_count": 0,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "record-aligned-1.1"
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("deployment database list --deployment-id=%s", deploymentId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": true,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "neo4j",
				"node_count": 0,
				"relationship_count": 0,
				"requested_primaries_count": 1,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "block-block-1.1"
			},
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": false,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "system",
				"node_count": null,
				"relationship_count": null,
				"requested_primaries_count": 0,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "record-aligned-1.1"
			}
		]
	}`)
}

func TestListDeploymentDatabaseWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/databases", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": [
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": true,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "50BE51F6E095D716850AA6FE7E9E7882677D21D7BB84658A1E171A55F24B143C",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "neo4j",
				"node_count": 0,
				"relationship_count": 0,
				"requested_primaries_count": 1,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "block-block-1.1"
			},
			{
				"access": "read-write",
				"aliases": [],
				"creation_time": "2025-11-10T13:57:24Z",
				"current_primaries_count": 1,
				"current_secondaries_count": 0,
				"default": false,
				"deployment_id": "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2",
				"id": "7D80871B447965646A0AE2AE844719308EFE00CBB0F9195FB0F3E12FA3E7D018",
				"last_start_time": "2025-11-10T13:57:24Z",
				"name": "system",
				"node_count": null,
				"relationship_count": null,
				"requested_primaries_count": 0,
				"requested_secondaries_count": 0,
				"requested_status": "online",
				"store": "record-aligned-1.1"
			}
		]
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "table")
	helper.ExecuteCommand(fmt.Sprintf("deployment database list --deployment-id=%s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOut(`
┌────────┬────────────┬─────────┬──────────────────────┬──────────────────────┬────────────┬────────────────────┬────────────────────┐
│ NAME   │ ACCESS     │ DEFAULT │ CREATION_TIME        │ LAST_START_TIME      │ NODE_COUNT │ RELATIONSHIP_COUNT │ STORE              │
├────────┼────────────┼─────────┼──────────────────────┼──────────────────────┼────────────┼────────────────────┼────────────────────┤
│ neo4j  │ read-write │ true    │ 2025-11-10T13:57:24Z │ 2025-11-10T13:57:24Z │ 0          │ 0                  │ block-block-1.1    │
│ system │ read-write │ false   │ 2025-11-10T13:57:24Z │ 2025-11-10T13:57:24Z │            │                    │ record-aligned-1.1 │
└────────┴────────────┴─────────┴──────────────────────┴──────────────────────┴────────────┴────────────────────┴────────────────────┘
	`)
}

func TestListDeploymentDatabasesWithNoData(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	organizationId := "81e4ae5c-171b-4700-b243-8d1dd34f7321"
	projectId := "ef7faf53-fb7e-4994-8d0f-64ae56e91c42"
	deploymentId := "9a1e6181-7d0b-48a2-bc2b-4250c36b5cc2"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/fleet-manager/deployments/%s/databases", organizationId, projectId, deploymentId), http.StatusOK, `{
		"data": []
	}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetConfigValue("output", "json")
	helper.ExecuteCommand(fmt.Sprintf("deployment database list --deployment-id=%s --organization-id=%s --project-id=%s", deploymentId, organizationId, projectId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{"data": []}`)
}
