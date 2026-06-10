// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestUpdateGraphQLDataApiFlagsValidation(t *testing.T) {
	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"
	orgProjectFlags := fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID)

	tests := map[string]struct {
		executedCommand string
		expectedError   string
		setupMocks      func(helper *testutils.AuraTestHelper)
	}{
		"provide only one type defs flag": {
			executedCommand: fmt.Sprintf("graphql update --format json --instance-id %s --type-definitions bla --type-definitions-file blabla %s --rw", instanceId, dataApiId),
			expectedError:   "Error: if any flags in the group [type-definitions type-definitions-file] are set none of the others can be; [type-definitions type-definitions-file] were all set",
		},
		"invalid type defs": {
			executedCommand: fmt.Sprintf("graphql update --format json --instance-id %s --type-definitions bla %s %s --rw", instanceId, dataApiId, orgProjectFlags),
			expectedError:   "Error: provided type definitions are not valid base64",
			setupMocks: func(helper *testutils.AuraTestHelper) {
				registerProjectsMock(helper)
				helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()
			if tt.setupMocks != nil {
				tt.setupMocks(&helper)
			}
			helper.ExecuteCommand(tt.executedCommand)
			helper.AssertErr(tt.expectedError)
		})
	}
}

func TestUpdateGraphQLDataApiWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	dataApiId := "75a234b5"
	name := "my-data-api-2"

	registerProjectsMock(&helper)
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{
		"data": {
			"id": "afdb4e9d",
			"name": "friendly-name-4",
			"status": "ready",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("graphql update --format json --instance-id %s --name %s %s\"\n\" --organization-id %s --project-id %s --rw", instanceId, name, dataApiId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
}

func TestUpdateGraphQLDataApiWithResponse(t *testing.T) {
	instanceId := "2f49c2b3"
	dataApiId := "75a234b5"
	name := "my-data-api-2"
	typeDefs := "dHlwZS=="

	mockResponse := `{
		"data": {
			"id": "afdb4e9d",
			"name": "friendly-name-4",
			"status": "ready",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
		}
	}`
	expectedResponse := `{
		"data": {
			"id": "afdb4e9d",
			"name": "friendly-name-4",
			"status": "ready",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
        }
	}`

	orgProjectFlags := fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID)

	tests := map[string]struct {
		mockResponse        string
		executeCommand      string
		expectedRequestBody string
		expectedResponse    string
	}{
		"update the name": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --name %s %s %s --rw", instanceId, name, dataApiId, orgProjectFlags),
			expectedRequestBody: `{"name":"my-data-api-2"}`,
			expectedResponse:    expectedResponse,
		}, "update the service account": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --service-account read_only %s %s --rw", instanceId, dataApiId, orgProjectFlags),
			expectedRequestBody: `{"aura_instance":{"service_account":"read_only"}}`,
			expectedResponse:    expectedResponse,
		}, "update the typeDefs": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --type-definitions %s %s %s --rw", instanceId, typeDefs, dataApiId, orgProjectFlags),
			expectedRequestBody: `{"type_definitions":"dHlwZS=="}`,
			expectedResponse:    expectedResponse,
		}, "update all possible values in one request": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --service-account read_write --type-definitions %s --name %s %s %s --rw", instanceId, typeDefs, name, dataApiId, orgProjectFlags),
			expectedRequestBody: `{"aura_instance":{"service_account":"read_write"},"name":"my-data-api-2","type_definitions":"dHlwZS=="}`,
			expectedResponse:    expectedResponse,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)
			helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, tt.mockResponse)

			helper.ExecuteCommand(tt.executeCommand)

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPatch)
			mockHandler.AssertCalledWithBody(tt.expectedRequestBody)

			helper.AssertOutJson(tt.expectedResponse)
		})
	}
}
