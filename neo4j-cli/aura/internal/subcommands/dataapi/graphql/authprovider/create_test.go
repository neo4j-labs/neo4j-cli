// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestCreateAuthProviderFlagsValidation(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetConfigValue("aura.beta-enabled", true)

	instanceId := "2f49c2b3"
	dataApiId := "23ea345a"
	name := "my-key-1"

	tests := map[string]struct {
		executedCommand string
		expectedError   string
	}{
		"missing all create flags": {
			executedCommand: fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s", instanceId, dataApiId),
			expectedError:   "Error: required flag(s) \"name\", \"type\" not set",
		},
		"missing name flag": {
			executedCommand: fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --type api-key", instanceId, dataApiId),
			expectedError:   "Error: required flag(s) \"name\" not set",
		},
		"missing type flag": {
			executedCommand: fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --disabled", instanceId, dataApiId, name),
			expectedError:   "Error: required flag(s) \"type\" not set",
		},
		"non-existing type flag": {
			executedCommand: fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --type bla --disabled", instanceId, dataApiId, name),
			expectedError:   `Error: invalid argument "bla" for "--type" flag: must be one of "api-key" or "jwks"`,
		},
		"missing url flag for jwks": {
			executedCommand: fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --type jwks --disabled", instanceId, dataApiId, name),
			expectedError:   "Error: required flag(s) \"url\" not set",
		},
		"can not set url flag for api-key": {
			executedCommand: fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --type api-key --url http://test.com/abc", instanceId, dataApiId, name),
			expectedError:   "Error: url flag can not be set for authentication provider type 'api-key'",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper.ExecuteCommand(tt.executedCommand)
			helper.AssertErr(tt.expectedError)
		})
	}
}

func TestCreateAuthProviderWithResponse(t *testing.T) {
	instanceId := "2f49c2b3"
	dataApiId := "23ea345a"
	nameApiKey := "my-key-2"
	nameJwks := "my-jwks-2"
	url := "https://test.com/.well-known/jwks.json"

	mockResponseApiKey := `{
		"data": {
			"id": "1ad1b794-e40e-41f7-8e8c-5638130317ed",
			"name": "my-key-2",
			"type": "api-key",
			"enabled": true,
			"key": "ublHwKxm2ylsc1HlkuL8NAcMfZnEVP1g"
		}
	}`
	mockResponseJwks := `{
		"data": {
			"id": "a3435b3-e40e-41f7-8e8c-5638130319aa",
			"name": "my-jwks-2",
			"type": "jwks",
			"enabled": false,
			"url": "https://test.com/.well-known/jwks.json"
		}
	}`

	expectedResponseJsonApiKey := `###############################
# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.
###############################
{
	"data": {
		"enabled": true,
		"id": "1ad1b794-e40e-41f7-8e8c-5638130317ed",
		"key": "ublHwKxm2ylsc1HlkuL8NAcMfZnEVP1g",
		"name": "my-key-2",
		"type": "api-key"
	}
}`
	expectedResponseTableApiKey := `###############################
# It is important to store the created API key! If you lose your API key, you will need to create a new Authentication provider. This will not result in any loss of data.
###############################
┌──────────────────────────────────────┬──────────┬─────────┬─────────┬──────────────────────────────────┬─────┐
│ ID                                   │ NAME     │ TYPE    │ ENABLED │ KEY                              │ URL │
├──────────────────────────────────────┼──────────┼─────────┼─────────┼──────────────────────────────────┼─────┤
│ 1ad1b794-e40e-41f7-8e8c-5638130317ed │ my-key-2 │ api-key │ true    │ ublHwKxm2ylsc1HlkuL8NAcMfZnEVP1g │     │
└──────────────────────────────────────┴──────────┴─────────┴─────────┴──────────────────────────────────┴─────┘
		`
	expectedResponseJsonJwks := `{
	"data": {
		"enabled": false,
		"id": "a3435b3-e40e-41f7-8e8c-5638130319aa",
		"name": "my-jwks-2",
		"type": "jwks",
		"url": "https://test.com/.well-known/jwks.json"
	}
}`
	expectedResponseTableJwks := `
┌─────────────────────────────────────┬───────────┬──────┬─────────┬─────┬────────────────────────────────────────┐
│ ID                                  │ NAME      │ TYPE │ ENABLED │ KEY │ URL                                    │
├─────────────────────────────────────┼───────────┼──────┼─────────┼─────┼────────────────────────────────────────┤
│ a3435b3-e40e-41f7-8e8c-5638130319aa │ my-jwks-2 │ jwks │ false   │     │ https://test.com/.well-known/jwks.json │
└─────────────────────────────────────┴───────────┴──────┴─────────┴─────┴────────────────────────────────────────┘
			`

	tests := map[string]struct {
		mockResponse        string
		executeCommand      string
		expectedRequestBody string
		expectedResponse    string
	}{
		"create api-key only with name": {
			mockResponse:        mockResponseApiKey,
			executeCommand:      fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --type api-key", instanceId, dataApiId, nameApiKey),
			expectedRequestBody: `{"enabled":true,"name":"my-key-2","type":"api-key"}`,
			expectedResponse:    expectedResponseJsonApiKey,
		},
		"create api-key with name and disabled flag": {
			mockResponse:        mockResponseApiKey,
			executeCommand:      fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --type api-key --disabled", instanceId, dataApiId, nameApiKey),
			expectedRequestBody: `{"enabled":false,"name":"my-key-2","type":"api-key"}`,
			expectedResponse:    expectedResponseJsonApiKey,
		},
		"create api-key with name and disabled flag response as table": {
			mockResponse:        mockResponseApiKey,
			executeCommand:      fmt.Sprintf("data-api graphql auth-provider create --format table --instance-id %s --data-api-id %s --name %s --type api-key --disabled", instanceId, dataApiId, nameApiKey),
			expectedRequestBody: `{"enabled":false,"name":"my-key-2","type":"api-key"}`,
			expectedResponse:    expectedResponseTableApiKey,
		},
		"create jwks only with name and url": {
			mockResponse:        mockResponseJwks,
			executeCommand:      fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --url %s --type jwks", instanceId, dataApiId, nameJwks, url),
			expectedRequestBody: `{"enabled":true,"name":"my-jwks-2","type":"jwks","url":"https://test.com/.well-known/jwks.json"}`,
			expectedResponse:    expectedResponseJsonJwks,
		},
		"create jwks with name and url and disabled flag": {
			mockResponse:        mockResponseJwks,
			executeCommand:      fmt.Sprintf("data-api graphql auth-provider create --instance-id %s --data-api-id %s --name %s --url %s --type jwks --disabled", instanceId, dataApiId, nameJwks, url),
			expectedRequestBody: `{"enabled":false,"name":"my-jwks-2","type":"jwks","url":"https://test.com/.well-known/jwks.json"}`,
			expectedResponse:    expectedResponseJsonJwks,
		},
		"create jwks with name and url response as table": {
			mockResponse:        mockResponseJwks,
			executeCommand:      fmt.Sprintf("data-api graphql auth-provider create --format table --instance-id %s --data-api-id %s --name %s --url %s --type jwks", instanceId, dataApiId, nameJwks, url),
			expectedRequestBody: `{"enabled":true,"name":"my-jwks-2","type":"jwks","url":"https://test.com/.well-known/jwks.json"}`,
			expectedResponse:    expectedResponseTableJwks,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetConfigValue("aura.beta-enabled", true)

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql/%s/auth-providers", instanceId, dataApiId), http.StatusAccepted, tt.mockResponse)

			helper.ExecuteCommand(tt.executeCommand)

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodPost)
			mockHandler.AssertCalledWithBody(tt.expectedRequestBody)

			helper.AssertOut(tt.expectedResponse)
		})
	}
}
