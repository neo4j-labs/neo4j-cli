// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const testDeleteInstanceID = "2f49c2b3"

// instanceGetBody returns a minimal GET /instances/{id} response body with the
// given tenant_id. Used to satisfy pre-flight ownership checks in mutating
// command tests.
func instanceGetBody(id, tenantID string) string {
	return fmt.Sprintf(`{
		"data": {
			"id": %q,
			"name": "Production",
			"status": "running",
			"tenant_id": %q,
			"cloud_provider": "gcp",
			"connection_url": "YOUR_CONNECTION_URL",
			"region": "europe-west1",
			"type": "enterprise-db",
			"memory": "8GB"
		}
	}`, id, tenantID)
}

func TestDeleteInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID

	// Single mock for /v1/instances/{id}: first call is GET (pre-flight), second is DELETE.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "deleting",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"project_id": "YOUR_TENANT_ID",
		"region": "europe-west1",
		"status": "deleting",
		"type": "enterprise-db"
	  }
	}`)
}

func TestDeleteInstanceWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testListOrgID, testListProjectID)
	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "deleting",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --rw --yes --force", instanceId))

	mockHandler.AssertCalledTimes(2)
	helper.AsssertOk()
}

func TestDeleteInstanceMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --rw", testDeleteInstanceID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestDeleteInstanceMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --rw", testDeleteInstanceID, testListOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestDeleteInstanceProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testListOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id unknown-project --rw", testDeleteInstanceID, testListOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testListOrgID)
}

func TestDeleteInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID

	// Instance belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find instance %s in project %s", instanceId, testListProjectID))
	helper.AssertUsageNotShown()
}

func TestDeleteInstanceWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	// Single mock: GET (pre-flight ownership check) then DELETE.
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{
		"data": {
		  "id": "2f49c2b3",
		  "name": "Production",
		  "status": "deleting",
		  "connection_url": "YOUR_CONNECTION_URL",
		  "tenant_id": "YOUR_TENANT_ID",
		  "cloud_provider": "gcp",
		  "memory": "8GB",
		  "region": "europe-west1",
		  "type": "enterprise-db"
		}
	  }`)

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s\"\n\" --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteInstanceError(t *testing.T) {
	testCases := []struct {
		statusCode    int
		expectedError string
		returnBody    string
	}{
		{
			statusCode: http.StatusNotFound,
			expectedError: `Error: [
	DB not found: 24d18db5
]`,
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
			statusCode: http.StatusConflict,
			expectedError: `Error: [
	The database is current undergoing an operation: resuming
]`,
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

			instanceId := testDeleteInstanceID

			// First call GET (pre-flight OK), second call DELETE (error).
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
			mockHandler.AddResponse(testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

			mockHandler.AssertCalledTimes(2)
			mockHandler.AssertCalledWithMethod(http.MethodDelete)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}

func TestDeleteInstanceConfirmGate_NonTTYWithoutFlags_Exit2(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

	err := helper.ExecuteCommandE(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("error %q missing 'pass both --yes and --force'", err.Error())
	}
	mockHandler.AssertCalledTimes(1) // pre-flight GET only, no DELETE
}

func TestDeleteInstanceConfirmGate_NonTTYWithBothFlags_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+instanceId+`", "status": "deleting"}}`)

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
}

func TestDeleteInstanceConfirmGate_TTYAnswerY_Proceeds(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("y\n")
	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))
	mockHandler.AddResponse(http.StatusAccepted, `{"data": {"id": "`+instanceId+`", "status": "deleting"}}`)

	helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithMethod(http.MethodDelete)
	helper.AssertErrContainsStrings([]string{"Delete instance"})
}

func TestDeleteInstanceConfirmGate_TTYAnswerN_Cancels(t *testing.T) {
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetStdin("N\n")
	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testListProjectID))

	err := helper.ExecuteCommandE(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID))

	if err != nil {
		t.Fatalf("expected nil (exit 0) on cancel, got %v", err)
	}
	mockHandler.AssertCalledTimes(1) // pre-flight GET only
	helper.AssertErrContainsStrings([]string{"cancelled."})
}
