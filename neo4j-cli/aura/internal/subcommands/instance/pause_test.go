// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestPauseInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Pre-flight GET for ownership check.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

	// Actual POST pause.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/pause", instanceId), http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "pausing",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "pausing",
		"type": "enterprise-db"
	  }
	}`)
}

func TestPauseInstanceWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testListOrgID, testListProjectID)
	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/pause", instanceId), http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "pausing",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s --rw", instanceId))

	mockHandler.AssertCalledTimes(1)
	helper.AsssertOk()
}

func TestPauseInstanceMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s --rw", "2f49c2b3"))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestPauseInstanceMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s --organization-id %s --rw", "2f49c2b3", testListOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestPauseInstanceProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s --organization-id %s --project-id unknown-project --rw", "2f49c2b3", testListOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testListOrgID)
}

func TestPauseInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testListProjectID))
	helper.AssertUsageNotShown()
}

func TestPauseInstanceWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Pre-flight GET for ownership check.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

	// Actual POST pause.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/pause", instanceId), http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "pausing",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s\"\n\" --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
}

func TestPauseInstanceError(t *testing.T) {
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

			// Pre-flight GET succeeds.
			helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

			// Actual POST fails.
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/pause", instanceId), testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf(`instance pause %s --organization-id %s --project-id %s --rw`, instanceId, testListOrgID, testListProjectID))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPost)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}
