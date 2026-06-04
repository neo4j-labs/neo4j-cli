// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListGraphQLDataApis(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	instanceId := "2f49c2b3"
	registerProjectsMock(&helper)
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId), http.StatusOK, `{
		"data": [
			{
				"id": "7261d20a",
				"name": "friendly-name",
				"status": "creating",
				"url": "https://23423.453489590fdsgs34.test.com/graphql"
			}
		]
	}`)

	helper.ExecuteCommand(fmt.Sprintf("graphql list --instance-id %s --organization-id %s --project-id %s", instanceId, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"id": "7261d20a",
				"name": "friendly-name",
				"status": "creating",
				"url": "https://23423.453489590fdsgs34.test.com/graphql"
			}
		]
	}
	`)
}

func TestListGraphQLDataApisWithCredentialFlag(t *testing.T) {
	instanceId := "2f49c2b3"

	for _, tc := range []struct {
		name    string
		command string
	}{
		{
			name:    "--credential flag",
			command: fmt.Sprintf("graphql list --instance-id %s --credential named-cred", instanceId),
		},
		{
			name:    "-c shorthand",
			command: fmt.Sprintf("graphql list --instance-id %s -c named-cred", instanceId),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetCredentialsValue("aura", map[string]interface{}{
				"credentials": []map[string]interface{}{
					{
						"name":          "named-cred",
						"client-id":     "named-client-id",
						"client-secret": "named-client-secret",
						"access-token":  "",
						"token-expiry":  0,
					},
				},
				"default-credential": "",
			})

			registerProjectsMock(&helper)
			helper.NewRequestHandlerMock(fmt.Sprintf("/v1/instances/%s", instanceId), http.StatusOK, instanceGetBody(instanceId, testProjectID))
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1beta5/instances/%s/data-apis/graphql", instanceId), http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command + fmt.Sprintf(" --organization-id %s --project-id %s", testOrgID, testProjectID))

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
