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

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"
	sourceId := "191b0da2"

	helper.NewRequestHandlerMock(fmt.Sprintf("GET /v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

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

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id %s --organization-id %s --project-id %s --rw", instanceId, sourceId, testListOrgID, testListProjectID))
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
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "overwriting",
		"type": "enterprise-db"
	  }
	}`)
}

func TestOverwriteWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"
	sourceId := "191b0da2"

	helper.NewRequestHandlerMock(fmt.Sprintf("GET /v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

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

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite --source-instance-id %s --organization-id %s --project-id %s --rw %s\"\n\"", sourceId, testListOrgID, testListProjectID, instanceId))
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
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "overwriting",
		"type": "enterprise-db"
	  }
	}`)
}

func TestOverwriteFromSnapshot(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"
	sourceId := "191b0da2"
	snapshotId := "3e5e6e27-bf0a-4898-abb8-5f3050cac418"

	helper.NewRequestHandlerMock(fmt.Sprintf("GET /v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

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

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id %s --source-snapshot-id %s --organization-id %s --project-id %s --rw", instanceId, sourceId, snapshotId, testListOrgID, testListProjectID))

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
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "overwriting",
		"type": "enterprise-db"
	  }
	}`)
}

func TestOverwriteWithWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

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

	// Pre-flight ownership check stays on the v1 flat instance path.
	preflightMock := helper.NewRequestHandlerMock("GET /v1/instances/2f49c2b3", http.StatusOK, instanceGetBody(instanceId, testListProjectID))

	// Readiness polling targets the v2beta1 org/project-scoped instance path.
	pollMock := helper.NewRequestHandlerMock(fmt.Sprintf("GET /v2beta1/organizations/%s/projects/%s/instances/2f49c2b3", testListOrgID, testListProjectID), http.StatusOK, `{
		"data": {
			"id": "2f49c2b3",
			"legacy_status": "overwriting"
		}
	}`).AddResponse(http.StatusOK, `{
		"data": {
			"id": "2f49c2b3",
			"legacy_status": "ready"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id %s --organization-id %s --project-id %s --wait --rw", instanceId, sourceId, testListOrgID, testListProjectID))

	postMock.AssertCalledTimes(1)
	postMock.AssertCalledWithBody(`{
		"source_instance_id": "191b0da2"
	  }`)

	preflightMock.AssertCalledTimes(1)
	pollMock.AssertCalledTimes(2)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "overwriting",
		"type": "enterprise-db"
	  }
	}`)
	helper.AssertErrContainsStrings([]string{
		"Waiting for instance to be ready...",
		"Instance Status: ready",
	})
}

func TestOverwriteMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("instance overwrite 2f49c2b3 --source-instance-id 191b0da2 --rw")

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestOverwriteMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite 2f49c2b3 --source-instance-id 191b0da2 --organization-id %s --rw", testListOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestOverwriteProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite 2f49c2b3 --source-instance-id 191b0da2 --organization-id %s --project-id unknown-project --rw", testListOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testListOrgID)
}

func TestOverwriteInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("GET /v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance overwrite %s --source-instance-id 191b0da2 --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testListProjectID))
	helper.AssertUsageNotShown()
}
