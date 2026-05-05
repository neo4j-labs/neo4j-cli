// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateGraphQLDataApiFlagsValidation(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	instanceId := "2f49c2b3"
	instanceUsername := "neo4j"
	instancePassword := "dfjglhssdopfrow"
	name := "my-data-api-1"
	typeDefs := "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwp9"
	invalidBase64TypeDefs := "df"
	typeDefsFile := "../../../test/assets/typeDefs.graphql"
	invalidTypeDefsFile := "../invalid/typeDefs.graphql"

	tests := map[string]struct {
		executedCommand string
		expectedError   string
	}{
		"missing almost all flags": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --type-definitions %s", instanceId, typeDefs),
			expectedError:   "Error: required flag(s) \"instance-password\", \"instance-username\", \"name\" not set",
		},
		"missing any type defs flag": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --instance-password %s --name %s ", instanceId, instanceUsername, instancePassword, name),
			expectedError:   "Error: at least one of the flags in the group [type-definitions type-definitions-file] is required",
		},
		"only one type defs flag can be provided": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --instance-password %s --name %s --type-definitions %s --type-definitions-file %s", instanceId, instanceUsername, instancePassword, name, typeDefs, typeDefsFile),
			expectedError:   "Error: if any flags in the group [type-definitions type-definitions-file] are set none of the others can be; [type-definitions type-definitions-file] were all set",
		},
		"missing instance password flag": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --name %s --type-definitions %s", instanceId, instanceUsername, name, typeDefs),
			expectedError:   "Error: required flag(s) \"instance-password\" not set",
		},
		"missing instance username flag": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-password %s --name %s --type-definitions %s", instanceId, instancePassword, name, typeDefs),
			expectedError:   "Error: required flag(s) \"instance-username\" not set",
		},
		"missing name flag": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --instance-password %s --type-definitions %s", instanceId, instanceUsername, instancePassword, typeDefs),
			expectedError:   "Error: required flag(s) \"name\" not set",
		},
		"invalid base64 for type defs": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --instance-password %s --name %s --type-definitions %s", instanceId, instanceUsername, instancePassword, name, invalidBase64TypeDefs),
			expectedError:   "Error: provided type definitions are not valid base64",
		},
		"invalid type defs file": {
			executedCommand: fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --instance-password %s --name %s --type-definitions-file %s", instanceId, instanceUsername, instancePassword, name, invalidTypeDefsFile),
			expectedError:   "Error: type definitions file '../invalid/typeDefs.graphql' does not exist",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper.ExecuteCommand(tt.executedCommand)
			helper.AssertErr(tt.expectedError)
		})
	}
}

func TestCreateGraphQLDataApiWithResponse(t *testing.T) {
	instanceId := "2f49c2b3"
	instanceUsername := "neo4j"
	instancePassword := "dfjglhssdopfrow"
	name := "my-data-api-1"
	typeDefsEncoded := "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="

	mockResponse := `{
		"data": {
			"id": "2f49c2b3",
			"name": "my-data-api-1",
			"status": "creating",
			"url": "https://2f49c2b3.28be6e4d8d3e8360197cb6c1fa1d25d1.graphql.neo4j-dev.io/graphql",
			"authentication_providers": [
				{
					"id": "1ad1b794-e40e-41f7-8e8c-5638130317ed",
					"name": "default",
					"type": "api-key",
					"enabled": true,
					"key": "ublHwKxm2ylsc1HlkuL8NAcMfZnEVP1g"
				}
			]
		}
	}`

	expectedResponseJson := `###############################
# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.
###############################
{
	"data": {
		"authentication_providers": [
			{
				"enabled": true,
				"id": "1ad1b794-e40e-41f7-8e8c-5638130317ed",
				"key": "ublHwKxm2ylsc1HlkuL8NAcMfZnEVP1g",
				"name": "default",
				"type": "api-key"
			}
		],
		"id": "2f49c2b3",
		"name": "my-data-api-1",
		"status": "creating",
		"url": "https://2f49c2b3.28be6e4d8d3e8360197cb6c1fa1d25d1.graphql.neo4j-dev.io/graphql"
	}
}`
	expectedResponseTable := `###############################
# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.
###############################
┌──────────┬───────────────┬──────────┬────────────────────────────────────────────────────────────────────────────────┬───────────────────────────────────────────────────┐
│ ID       │ NAME          │ STATUS   │ URL                                                                            │ AUTHENTICATION_PROVIDERS                          │
├──────────┼───────────────┼──────────┼────────────────────────────────────────────────────────────────────────────────┼───────────────────────────────────────────────────┤
│ 2f49c2b3 │ my-data-api-1 │ creating │ https://2f49c2b3.28be6e4d8d3e8360197cb6c1fa1d25d1.graphql.neo4j-dev.io/graphql │ [                                                 │
│          │               │          │                                                                                │   {                                               │
│          │               │          │                                                                                │     "enabled": true,                              │
│          │               │          │                                                                                │     "id": "1ad1b794-e40e-41f7-8e8c-5638130317ed", │
│          │               │          │                                                                                │     "key": "ublHwKxm2ylsc1HlkuL8NAcMfZnEVP1g",    │
│          │               │          │                                                                                │     "name": "default",                            │
│          │               │          │                                                                                │     "type": "api-key"                             │
│          │               │          │                                                                                │   }                                               │
│          │               │          │                                                                                │ ]                                                 │
└──────────┴───────────────┴──────────┴────────────────────────────────────────────────────────────────────────────────┴───────────────────────────────────────────────────┘
	`

	tests := map[string]struct {
		mockResponse        string
		executeCommand      string
		expectedRequestBody string
		expectedResponse    string
	}{
		"create with default auth provider": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("data-api graphql create --instance-id %s --instance-username %s --instance-password %s --name %s --type-definitions %s", instanceId, instanceUsername, instancePassword, name, typeDefsEncoded),
			expectedRequestBody: `{"aura_instance":{"password":"dfjglhssdopfrow","username":"neo4j"},"name":"my-data-api-1","security":{"authentication_providers":[{"enabled":true,"name":"default","type":"api-key"}]},"type_definitions":"dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="}`,
			expectedResponse:    expectedResponseJson,
		}, "create with default auth provider and output as table": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("data-api graphql create --format table --instance-id %s --instance-username %s --instance-password %s --name %s --type-definitions %s ", instanceId, instanceUsername, instancePassword, name, typeDefsEncoded),
			expectedRequestBody: `{"aura_instance":{"password":"dfjglhssdopfrow","username":"neo4j"},"name":"my-data-api-1","security":{"authentication_providers":[{"enabled":true,"name":"default","type":"api-key"}]},"type_definitions":"dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="}`,
			expectedResponse:    expectedResponseTable,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetConfigValue("aura.beta-enabled", true)

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId), http.StatusAccepted, tt.mockResponse)

			helper.ExecuteCommand(tt.executeCommand)

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPost)
			mockHandler.AssertCalledWithBody(tt.expectedRequestBody)

			helper.AssertOut(tt.expectedResponse)
		})
	}
}
