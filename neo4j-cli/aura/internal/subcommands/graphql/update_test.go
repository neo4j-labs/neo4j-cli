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
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	dataApiId := "afdb4e9d"

	tests := map[string]struct {
		executedCommand string
		expectedError   string
	}{
		"provide only one type defs flag": {
			executedCommand: fmt.Sprintf("graphql update --format json --instance-id %s --type-definitions bla --type-definitions-file blabla %s --rw", instanceId, dataApiId),
			expectedError:   "Error: if any flags in the group [type-definitions type-definitions-file] are set none of the others can be; [type-definitions type-definitions-file] were all set",
		},
		"invalid type defs": {
			executedCommand: fmt.Sprintf("graphql update --format json --instance-id %s --type-definitions bla %s --rw", instanceId, dataApiId),
			expectedError:   "Error: provided type definitions are not valid base64",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
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

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, `{
		"data": {
			"id": "afdb4e9d",
			"name": "friendly-name-4",
			"status": "ready",
			"url": "https://afdb4e9d.28be6e4d8d3e836019.graphql.neo4j.io/graphql"
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("graphql update --format json --instance-id %s --name %s %s\"\n\" --rw", instanceId, name, dataApiId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
}

func TestUpdateGraphQLDataApiWithResponse(t *testing.T) {
	instanceId := "2f49c2b3"
	dataApiId := "75a234b5"
	instanceUsername := "neo4j"
	instancePassword := "dfjglhssdopfrow"
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

	tests := map[string]struct {
		mockResponse        string
		executeCommand      string
		expectedRequestBody string
		expectedResponse    string
	}{
		"update the name": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --name %s %s --rw", instanceId, name, dataApiId),
			expectedRequestBody: `{"name":"my-data-api-2"}`,
			expectedResponse:    expectedResponse,
		}, "update the password": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --instance-password %s %s --rw", instanceId, instancePassword, dataApiId),
			expectedRequestBody: `{"aura_instance":{"password":"dfjglhssdopfrow"}}`,
			expectedResponse:    expectedResponse,
		}, "update the username": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --instance-username %s %s --rw", instanceId, instanceUsername, dataApiId),
			expectedRequestBody: `{"aura_instance":{"username":"neo4j"}}`,
			expectedResponse:    expectedResponse,
		}, "update the password and username": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --instance-password %s --instance-username %s %s --rw", instanceId, instancePassword, instanceUsername, dataApiId),
			expectedRequestBody: `{"aura_instance":{"password":"dfjglhssdopfrow","username":"neo4j"}}`,
			expectedResponse:    expectedResponse,
		}, "update the typeDefs": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --type-definitions %s %s --rw", instanceId, typeDefs, dataApiId),
			expectedRequestBody: `{"type_definitions":"dHlwZS=="}`,
			expectedResponse:    expectedResponse,
		}, "update all possible values in one request": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql update --format json --instance-id %s --instance-password %s --instance-username %s --type-definitions %s --name %s %s --rw", instanceId, instancePassword, instanceUsername, typeDefs, name, dataApiId),
			expectedRequestBody: `{"aura_instance":{"password":"dfjglhssdopfrow","username":"neo4j"},"name":"my-data-api-2","type_definitions":"dHlwZS=="}`,
			expectedResponse:    expectedResponse,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s", instanceId, dataApiId), http.StatusAccepted, tt.mockResponse)

			helper.ExecuteCommand(tt.executeCommand)

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPatch)
			mockHandler.AssertCalledWithBody(tt.expectedRequestBody)

			helper.AssertOutJson(tt.expectedResponse)
		})
	}
}
