// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestOverwriteFromInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	sourceId := "191b0da2"

	postMock := helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/overwrite", instanceId), http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "overwriting",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id %s --rw", instanceId, sourceId))
	postMock.AssertCalledTimes(1)
	postMock.AssertCalledWithBody(`{
		"source_instance_id": "191b0da2"
	  }`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"region": "europe-west1",
		"status": "overwriting",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "enterprise-db"
	  }
	}`)
}

func TestOverwriteFromSnapshot(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	sourceId := "191b0da2"
	snapshotId := "3e5e6e27-bf0a-4898-abb8-5f3050cac418"

	postMock := helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/overwrite", instanceId), http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "overwriting",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id %s --source-snapshot-id %s --rw", instanceId, sourceId, snapshotId))

	postMock.AssertCalledTimes(1)
	postMock.AssertCalledWithBody(`{
		"source_instance_id": "191b0da2","source_snapshot_id": "3e5e6e27-bf0a-4898-abb8-5f3050cac418"
	  }`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"region": "europe-west1",
		"status": "overwriting",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "enterprise-db"
	  }
	}`)
}

func TestOverwriteWithAwait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	sourceId := "191b0da2"

	postMock := helper.NewRequestHandlerMock(fmt.Sprintf("POST /v1/instances/%s/overwrite", instanceId), http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "overwriting",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	getMock := helper.NewRequestHandlerMock("GET /v1/instances/2f49c2b3", http.StatusOK, `{
		"data": {
			"id": "2f49c2b3",
			"status": "overwriting"
		}
	}`).AddResponse(http.StatusOK, `{
		"data": {
			"id": "2f49c2b3",
			"status": "ready"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id %s --await --rw", instanceId, sourceId))

	postMock.AssertCalledTimes(1)
	postMock.AssertCalledWithBody(`{
		"source_instance_id": "191b0da2"
	  }`)

	getMock.AssertCalledTimes(2)

	helper.AsssertOk()

	helper.AssertOut(`{
	"data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"region": "europe-west1",
		"status": "overwriting",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "enterprise-db"
	}
}
Waiting for instance to be ready...
Instance Status: ready
	  `)
}
