// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestUpdateMemory(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Single mock: GET (pre-flight) then PATCH.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
			"id": "2f49c2b3",
			"name": "Production",
			"status": "updating",
			"connection_url": "YOUR_CONNECTION_URL",
			"tenant_id": "YOUR_TENANT_ID",
			"cloud_provider": "gcp",
			"memory": "8GB",
			"region": "europe-west1",
			"type": "enterprise-db"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("instance update %s --memory 8GB --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	mockHandler.AssertCalledWithBody(`{"memory":"8GB"}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "updating",
		"type": "enterprise-db"
	  }
	}`)
}

func TestUpdateName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusOK, `{
		"data": {
			"id": "2f49c2b3",
			"name": "New Name",
			"status": "updating",
			"connection_url": "YOUR_CONNECTION_URL",
			"tenant_id": "YOUR_TENANT_ID",
			"cloud_provider": "gcp",
			"memory": "4GB",
			"region": "europe-west1",
			"type": "enterprise-db"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf(`instance update %s --name "New Name" --organization-id %s --project-id %s --rw`, instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	mockHandler.AssertCalledWithBody(`{"name":"New Name"}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "4GB",
		"name": "New Name",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "updating",
		"type": "enterprise-db"
	  }
	}`)
}

func TestUpdateMemoryAndName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
			"id": "2f49c2b3",
			"name": "New Name",
			"status": "updating",
			"connection_url": "YOUR_CONNECTION_URL",
			"tenant_id": "YOUR_TENANT_ID",
			"cloud_provider": "gcp",
			"memory": "8GB",
			"region": "europe-west1",
			"type": "enterprise-db"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf(`instance update %s --name "New Name" --memory 8GB --organization-id %s --project-id %s --rw`, instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	mockHandler.AssertCalledWithBody(`{"memory":"8GB","name":"New Name"}`)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "New Name",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "updating",
		"type": "enterprise-db"
	  }
	}`)
}

func TestUpdateErrorsWithNoFlags(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusAccepted, "")

	helper.ExecuteCommand(fmt.Sprintf(`instance update %s --rw`, instanceId))

	mockHandler.AssertCalledTimes(0)

	helper.AssertErr(`Error: at least one of the flags in the group [memory name] is required
`)
}

func TestUpdateInstanceWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testListOrgID, testListProjectID)
	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
			"id": "2f49c2b3",
			"name": "Production",
			"status": "updating",
			"connection_url": "YOUR_CONNECTION_URL",
			"tenant_id": "YOUR_TENANT_ID",
			"cloud_provider": "gcp",
			"memory": "8GB",
			"region": "europe-west1",
			"type": "enterprise-db"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("instance update %s --memory 8GB --rw", instanceId))

	mockHandler.AssertCalledTimes(2)
	helper.AsssertOk()
}

func TestUpdateInstanceMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance update %s --memory 8GB --rw", "2f49c2b3"))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestUpdateInstanceMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance update %s --memory 8GB --organization-id %s --rw", "2f49c2b3", testListOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestUpdateInstanceProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("instance update %s --memory 8GB --organization-id %s --project-id unknown-project --rw", "2f49c2b3", testListOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testListOrgID)
}

func TestUpdateInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance update %s --memory 8GB --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testListProjectID))
}

func TestUpdateInstanceError(t *testing.T) {
	testCases := []struct {
		statusCode    int
		expectedError string
		returnBody    string
	}{
		{
			statusCode:    http.StatusNotFound,
			expectedError: "Error: [DB not found: 24d18db5]",
			returnBody: `{
			"errors": [
			  {
				"message": "DB not found: 24d18db5",
				"reason": "db-not-found"
			  }
			]
		  }`,
		},
		{
			statusCode:    http.StatusConflict,
			expectedError: "Error: [The database is current undergoing an operation: resuming]",
			returnBody: `{
				"errors": [
				  {
					"message": "The database is current undergoing an operation: resuming",
					"reason": "ongoing-database-operation"
				  }
				]
			}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("StatusCode%d", testCase.statusCode), func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)

			instanceId := "2f49c2b3"

			// First call GET (pre-flight OK), second call PATCH (error).
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
			mockHandler.AddResponse(testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf(`instance update %s --name "New Name" --memory 8GB --organization-id %s --project-id %s --rw`, instanceId, testListOrgID, testListProjectID))

			mockHandler.AssertCalledTimes(2)
			mockHandler.AssertCalledWithMethod(http.MethodPatch)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}
