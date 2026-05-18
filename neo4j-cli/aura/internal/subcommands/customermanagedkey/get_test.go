// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetCustomerManagedKey(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		registerProjectsMock(&helper)

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s --organization-id %s --project-id %s", command, testCMKID, testOrgID, testProjectID))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)

		helper.AssertOutJson(`{
		  "data": {
			"cloud_provider": "aws",
			"created": "2024-01-31T14:06:57Z",
			"id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
			"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
			"name": "Instance01",
			"project_id": "YOUR_TENANT_ID",
			"region": "us-east-1",
			"status": "ready",
			"type": "enterprise-db"
		  }
		}`)
	}
}

func TestGetCustomerManagedKeyWithDefaultWorkspace(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
		registerProjectsMock(&helper)

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s", command, testCMKID))

		mockHandler.AssertCalledTimes(1)
		helper.AsssertOk()
	}
}

func TestGetCustomerManagedKeyMissingOrg(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s", command, testCMKID))

		mockHandler.AssertCalledTimes(0)
		helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
	}
}

func TestGetCustomerManagedKeyMissingProject(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s --organization-id %s", command, testCMKID, testOrgID))

		mockHandler.AssertCalledTimes(0)
		helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
	}
}

func TestGetCustomerManagedKeyProjectNotInOrg(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		helper.NewRequestHandlerMock(
			"/v2beta1/organizations/"+testOrgID+"/projects",
			http.StatusOK,
			`{"data": []}`,
		)

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s --organization-id %s --project-id unknown-project", command, testCMKID, testOrgID))

		mockHandler.AssertCalledTimes(0)
		helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
	}
}

func TestGetCustomerManagedKeyNotInProject(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		registerProjectsMock(&helper)

		// CMK belongs to a different project.
		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, "other-project-id"))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s --organization-id %s --project-id %s", command, testCMKID, testOrgID, testProjectID))

		mockHandler.AssertCalledTimes(1)
		helper.AssertErr(fmt.Sprintf("Error: could not find customer-managed-key %s in project %s", testCMKID, testProjectID))
	}
}

func TestGetCustomerManagedKeyNotFoundError(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		registerProjectsMock(&helper)

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusNotFound, fmt.Sprintf(`{
			"errors": [
				{
				"message": "Encryption Key not found: %s",
				"reason": "encryption-key-not-found"
				}
			]
			}`, testCMKID))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s --organization-id %s --project-id %s", command, testCMKID, testOrgID, testProjectID))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)

		helper.AssertErr(fmt.Sprintf("Error: [Encryption Key not found: %s]", testCMKID))
	}
}
