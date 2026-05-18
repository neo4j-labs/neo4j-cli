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

	instanceId := "2f49c2b3"

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
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(`{
	  "data": {
		"cloud_provider": "gcp",
		"connection_url": "YOUR_CONNECTION_URL",
		"id": "2f49c2b3",
		"memory": "8GB",
		"name": "Production",
		"region": "europe-west1",
		"status": "pausing",
		"tenant_id": "YOUR_TENANT_ID",
		"type": "enterprise-db"
	  }
	}`)
}

func TestPauseInstanceWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"

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

	helper.ExecuteCommand(fmt.Sprintf("instance pause %s\"\n\" --rw", instanceId))

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

			instanceId := "2f49c2b3"

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s/pause", instanceId), testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf(`instance pause %s --rw`, instanceId))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPost)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}
