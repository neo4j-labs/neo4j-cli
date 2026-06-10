// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateGraphQLDataApiFlagsValidation(t *testing.T) {
	instanceId := "2f49c2b3"
	name := "my-data-api-1"
	typeDefs := "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwp9"
	invalidBase64TypeDefs := "df"
	typeDefsFile := "../../../test/assets/typeDefs.graphql"
	invalidTypeDefsFile := "../invalid/typeDefs.graphql"
	orgProjectFlags := fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID)

	tests := map[string]struct {
		executedCommand string
		expectedError   string
		setupMocks      func(helper *testutils.AuraTestHelper)
	}{
		"missing any type defs flag": {
			executedCommand: fmt.Sprintf("graphql create --instance-id %s --name %s --memory 256MB --rw", instanceId, name),
			expectedError:   "Error: at least one of the flags in the group [type-definitions type-definitions-file] is required",
		},
		"only one type defs flag can be provided": {
			executedCommand: fmt.Sprintf("graphql create --instance-id %s --name %s --memory 256MB --type-definitions %s --type-definitions-file %s --rw", instanceId, name, typeDefs, typeDefsFile),
			expectedError:   "Error: if any flags in the group [type-definitions type-definitions-file] are set none of the others can be; [type-definitions type-definitions-file] were all set",
		},
		"invalid base64 for type defs": {
			executedCommand: fmt.Sprintf("graphql create --instance-id %s --name %s --memory 256MB --type-definitions %s %s --rw", instanceId, name, invalidBase64TypeDefs, orgProjectFlags),
			expectedError:   "Error: provided type definitions are not valid base64",
			setupMocks: func(helper *testutils.AuraTestHelper) {
				registerProjectsMock(helper)
				helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			},
		},
		"invalid type defs file": {
			executedCommand: fmt.Sprintf("graphql create --instance-id %s --name %s --memory 256MB --type-definitions-file %s %s --rw", instanceId, name, invalidTypeDefsFile, orgProjectFlags),
			expectedError:   "Error: type definitions file '../invalid/typeDefs.graphql' does not exist",
			setupMocks: func(helper *testutils.AuraTestHelper) {
				registerProjectsMock(helper)
				helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			},
		},
		"invalid service account value": {
			executedCommand: fmt.Sprintf("graphql create --instance-id %s --name %s --memory 256MB --type-definitions %s --service-account bad_value --rw", instanceId, name, typeDefs),
			expectedError:   `Error: invalid value for --service-account: "bad_value", must be one of: read_only, read_write`,
		},
		"invalid memory value": {
			executedCommand: fmt.Sprintf("graphql create --instance-id %s --name %s --memory 128MB --type-definitions %s --rw", instanceId, name, typeDefs),
			expectedError:   `Error: invalid value for --memory: "128MB", must be one of: 256MB, 512MB, 1024MB, 2048MB, 4096MB`,
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

func TestCreateGraphQLDataApiWithResponse(t *testing.T) {
	instanceId := "2f49c2b3"
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

	expectedResponseJson := `{
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
	expectedResponseTable := `┌──────────┬───────────────┬──────────┬────────────────────────────────────────────────────────────────────────────────┬───────────────────────────────────────────────────┐
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

	orgProjectFlags := fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID)

	tests := map[string]struct {
		mockResponse        string
		executeCommand      string
		expectedRequestBody string
		expectedResponse    string
	}{
		"create with default service account": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql create --instance-id %s --name %s --memory 256MB --type-definitions %s %s --rw", instanceId, name, typeDefsEncoded, orgProjectFlags),
			expectedRequestBody: `{"aura_instance":{"service_account":"read_write"},"memory":"256MB","name":"my-data-api-1","security":{"authentication_providers":[{"enabled":true,"name":"default","type":"api-key"}]},"type_definitions":"dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="}`,
			expectedResponse:    expectedResponseJson,
		}, "create with read_only service account and output as table": {
			mockResponse:        mockResponse,
			executeCommand:      fmt.Sprintf("graphql create --format table --instance-id %s --name %s --memory 512MB --service-account read_only --type-definitions %s %s --rw", instanceId, name, typeDefsEncoded, orgProjectFlags),
			expectedRequestBody: `{"aura_instance":{"service_account":"read_only"},"memory":"512MB","name":"my-data-api-1","security":{"authentication_providers":[{"enabled":true,"name":"default","type":"api-key"}]},"type_definitions":"dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="}`,
			expectedResponse:    expectedResponseTable,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)
			helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId), http.StatusAccepted, tt.mockResponse)

			helper.ExecuteCommand(tt.executeCommand)

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPost)
			mockHandler.AssertCalledWithBody(tt.expectedRequestBody)

			helper.AssertOut(tt.expectedResponse)
			helper.AssertErrContainsStrings([]string{
				"###############################",
				"# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.",
			})
		})
	}
}

func TestCreateGraphQLDataApi_AutoName(t *testing.T) {
	instanceId := "2f49c2b3"
	typeDefsEncoded := "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="

	listResponse := `{"data": [{"id": "aaa", "name": "GraphQL01", "status": "ready", "url": "https://example.com"}]}`
	createResponse := `{
		"data": {
			"id": "2f49c2b3",
			"name": "GraphQL02",
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

	orgProjectFlags := fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID)

	t.Run("auto-name skips taken name and picks next available", func(t *testing.T) {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		registerProjectsMock(&helper)
		helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
		mockHandler := helper.NewRequestHandlerMock(
			fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId),
			http.StatusOK,
			listResponse,
		)
		mockHandler.AddResponse(http.StatusAccepted, createResponse)

		helper.ExecuteCommand(fmt.Sprintf(
			"graphql create --instance-id %s --memory 256MB --type-definitions %s %s --rw",
			instanceId, typeDefsEncoded, orgProjectFlags,
		))

		mockHandler.AssertCalledTimes(2)
		mockHandler.AssertCalledWithMethod(http.MethodGet)
		mockHandler.AssertCalledWithMethod(http.MethodPost)
		mockHandler.AssertCalledWithBody(`{"aura_instance":{"service_account":"read_write"},"memory":"256MB","name":"GraphQL02","security":{"authentication_providers":[{"enabled":true,"name":"default","type":"api-key"}]},"type_definitions":"dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="}`)
	})

	t.Run("explicit --name skips list GET and posts exact name", func(t *testing.T) {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		registerProjectsMock(&helper)
		helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
		mockHandler := helper.NewRequestHandlerMock(
			fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId),
			http.StatusAccepted,
			createResponse,
		)

		helper.ExecuteCommand(fmt.Sprintf(
			"graphql create --instance-id %s --name my-api --memory 256MB --type-definitions %s %s --rw",
			instanceId, typeDefsEncoded, orgProjectFlags,
		))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodPost)
		mockHandler.AssertCalledWithBody(`{"aura_instance":{"service_account":"read_write"},"memory":"256MB","name":"my-api","security":{"authentication_providers":[{"enabled":true,"name":"default","type":"api-key"}]},"type_definitions":"dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="}`)
	})
}

func TestCreateGraphQLDataApi_RegistersApiKeySecrets(t *testing.T) {
	instanceId := "2f49c2b3"
	apiKey := "secretKeyValueForTeeRedactionGraphQL"

	mockResponse := fmt.Sprintf(`{
		"data": {
			"id": "2f49c2b3",
			"name": "my-data-api",
			"status": "creating",
			"url": "https://2f49c2b3.example.graphql.neo4j.io/graphql",
			"authentication_providers": [
				{
					"id": "1ad1b794-e40e-41f7-8e8c-5638130317ed",
					"name": "default",
					"type": "api-key",
					"enabled": true,
					"key": %q
				}
			]
		}
	}`, apiKey)

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	orgProjectFlags := fmt.Sprintf("--organization-id %s --project-id %s", testOrgID, testProjectID)
	registerProjectsMock(&helper)
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId), http.StatusAccepted, mockResponse)

	helper.ExecuteCommand(fmt.Sprintf(
		"graphql create --instance-id %s --name my-data-api --memory 256MB --type-definitions dHlwZSBNb3ZpZSB7IHRpdGxlOiBTdHJpbmcgfQ== %s --rw",
		instanceId, orgProjectFlags,
	))

	if clievents.RedactText(apiKey) != "***" {
		t.Errorf("expected API key to be registered as a secret value for tee redaction, but RedactText(%q) did not return ***", apiKey)
	}
}

// TestCreateGraphQLDataApi_StdoutIsValidJSON is the CLI-82 regression-pin
// for the banner: pre-fix, the "###" banner was emitted to stdout, which broke
// `--format json | jq`. Reverting the Pattern C fmt.Fprintln replacements to
// cmd.Println in graphql/create.go causes this test to fail.
func TestCreateGraphQLDataApi_StdoutIsValidJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
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

	registerProjectsMock(&helper)
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId), http.StatusAccepted, mockResponse)

	helper.ExecuteCommand(fmt.Sprintf("graphql create --instance-id %s --name my-data-api-1 --memory 256MB --type-definitions dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ== --organization-id %s --project-id %s --rw --format json", instanceId, testOrgID, testProjectID))

	helper.AssertOutIsValidJSON()
}
