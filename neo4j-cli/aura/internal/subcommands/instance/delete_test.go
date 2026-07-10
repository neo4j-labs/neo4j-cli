// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm/confirmtest"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const testDeleteInstanceID = "2f49c2b3"

func TestDeleteInstance(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusAccepted, `{
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

	mockHandler.AssertCalledTimes(1)
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

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusAccepted, `{
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

	mockHandler.AssertCalledTimes(1)
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

// TestDeleteInstanceNotInProject covers an instance outside the scoped project.
// The v2beta1 scoped delete path natively 404s; no tenant_id preflight is done.
func TestDeleteInstanceNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := testDeleteInstanceID

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusNotFound, fmt.Sprintf(`{
		"errors": [
			{
			"message": "DB not found: %s",
			"reason": "db-not-found"
			}
		]
	}`, instanceId))

	err := helper.ExecuteCommandE(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

	mockHandler.AssertCalledWithMethod(http.MethodDelete)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)
	require.Equal(t, "instance", ce.ResourceType)
	require.Equal(t, instanceId, ce.ResourceID)

	helper.AssertErr(fmt.Sprintf("Error: [\n\tDB not found: %s\n]", instanceId))
}

func TestDeleteInstanceWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	instanceId := "2f49c2b3"

	mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusAccepted, `{
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

	mockHandler.AssertCalledTimes(1)
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

			mockHandler := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw --yes --force", instanceId, testListOrgID, testListProjectID))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodDelete)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}

func TestDeleteInstanceConfirmGate(t *testing.T) {
	instanceId := testDeleteInstanceID
	base := fmt.Sprintf("instance delete %s --organization-id %s --project-id %s --rw", instanceId, testListOrgID, testListProjectID)
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "aura instance delete",
		NoFlagsArgs:   base,
		BothFlagsArgs: base + " --yes --force",
		ResourceLabel: "instance",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			helper := testutils.NewAuraTestHelper(t)
			t.Cleanup(helper.Close)
			registerProjectsMock(&helper)
			mock := helper.NewRequestHandlerMock(scopedInstancePath(instanceId), http.StatusAccepted, `{"data": {"id": "`+instanceId+`", "status": "deleting"}}`)
			helper.SetStdin(stdin)
			err := helper.ExecuteCommandE(args)
			return confirmtest.GateRunResult{Err: err, Stderr: helper.PrintErr(), Invoked: mock.CalledWithMethod(http.MethodDelete)}
		},
	})
}
