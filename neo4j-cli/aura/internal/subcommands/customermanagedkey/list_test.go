// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListCustomerManagedKeys(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		registerProjectsMock(&helper)

		mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusOK, `{
		"data": [
			{
				"id": "f15cc45b-1c29-44e8-911f-3ba719f70ed7",
				"name": "Production Key",
				"tenant_id": "YOUR_TENANT_ID"
			},
			{
				"id": "0d971cc4-f703-40fd-8c5c-f5ec134f6c84",
				"name": "Dev Key",
				"tenant_id": "YOUR_TENANT_ID"
			}
		]
		}`)

		helper.ExecuteCommand(fmt.Sprintf("%s list --organization-id %s --project-id %s", command, testOrgID, testProjectID))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)
		mockHandler.AssertCalledWithQueryParam("tenantId", testProjectID)

		helper.AssertOutJson(`{
			"data": [
				{
					"id": "f15cc45b-1c29-44e8-911f-3ba719f70ed7",
					"name": "Production Key",
					"project_id": "YOUR_TENANT_ID"
				},
				{
					"id": "0d971cc4-f703-40fd-8c5c-f5ec134f6c84",
					"name": "Dev Key",
					"project_id": "YOUR_TENANT_ID"
				}
			]
		}
		`)
	}
}

func TestListCustomerManagedKeysWithDefaultWorkspace(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
		registerProjectsMock(&helper)

		mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusOK, `{
		"data": [
			{
				"id": "f15cc45b-1c29-44e8-911f-3ba719f70ed7",
				"name": "Production Key",
				"tenant_id": "YOUR_TENANT_ID"
			}
		]
		}`)

		helper.ExecuteCommand(fmt.Sprintf("%s list", command))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)
		mockHandler.AssertCalledWithQueryParam("tenantId", testProjectID)

		helper.AssertOutJson(`{
			"data": [
				{
					"id": "f15cc45b-1c29-44e8-911f-3ba719f70ed7",
					"name": "Production Key",
					"project_id": "YOUR_TENANT_ID"
				}
			]
		}`)
	}
}

func TestListCustomerManagedKeysMissingOrg(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusOK, `{"data": []}`)

		helper.ExecuteCommand(fmt.Sprintf("%s list", command))

		mockHandler.AssertCalledTimes(0)
		helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
	}
}

func TestListCustomerManagedKeysMissingProject(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusOK, `{"data": []}`)

		helper.ExecuteCommand(fmt.Sprintf("%s list --organization-id %s", command, testOrgID))

		mockHandler.AssertCalledTimes(0)
		helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
	}
}

func TestListCustomerManagedKeysProjectNotInOrg(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.NewRequestHandlerMock(
			"/v2beta1/organizations/"+testOrgID+"/projects",
			http.StatusOK,
			`{"data": []}`,
		)

		mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusOK, `{"data": []}`)

		helper.ExecuteCommand(fmt.Sprintf("%s list --organization-id %s --project-id unknown-project", command, testOrgID))

		mockHandler.AssertCalledTimes(0)
		helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
	}
}

func TestListCustomerManagedKeysWithInvalidOutput(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.ExecuteCommand(fmt.Sprintf("%s list --format invalid", command))

		helper.AssertErr("Error: invalid format value specified: invalid")
	}
}

func TestListCustomerManagedKeysWithCredentialFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
	}{
		{name: "--credential flag", command: fmt.Sprintf("customer-managed-key list --organization-id %s --project-id %s --credential named-cred", testOrgID, testProjectID)},
		{name: "-c shorthand", command: fmt.Sprintf("customer-managed-key list --organization-id %s --project-id %s -c named-cred", testOrgID, testProjectID)},
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

			helper.NewRequestHandlerMock(
				"/v2beta1/organizations/"+testOrgID+"/projects",
				http.StatusOK,
				`{"data": [{"id": "`+testProjectID+`", "name": "Test Project"}]}`,
			)

			mockHandler := helper.NewRequestHandlerMock("/v1/customer-managed-keys", http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AsssertOk()
		})
	}
}
