// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestDeleteCustomerManagedKey(t *testing.T) {
	commands := []string{"customer-managed-key", "cmk"}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)

			// Single mock for /v1/customer-managed-keys/{id}: first call is GET (pre-flight), second is DELETE.
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))
			mockHandler.AddResponse(http.StatusNoContent, "")

			helper.ExecuteCommand(fmt.Sprintf("%s delete %s --organization-id %s --project-id %s --rw", command, testCMKID, testOrgID, testProjectID))

			mockHandler.AssertCalledTimes(2)
			mockHandler.AssertCalledWithMethod(http.MethodDelete)

			helper.AssertErrContainsStrings([]string{fmt.Sprintf("customer-managed-key %s deleted", testCMKID)})
			helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"deleted": true,
		"id": "%s"
	}
}`, testCMKID))
		})
	}
}

func TestDeleteCustomerManagedKeyWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))
	mockHandler.AddResponse(http.StatusNoContent, "")

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --rw", testCMKID))

	mockHandler.AssertCalledTimes(2)
	helper.AssertErrContainsStrings([]string{fmt.Sprintf("customer-managed-key %s deleted", testCMKID)})
}

func TestDeleteCustomerManagedKeyMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --rw", testCMKID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestDeleteCustomerManagedKeyMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --organization-id %s --rw", testCMKID, testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestDeleteCustomerManagedKeyProjectNotInOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": []}`,
	)

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --organization-id %s --project-id unknown-project --rw", testCMKID, testOrgID))

	helper.AssertErr("Error: could not find project unknown-project in organization " + testOrgID)
}

func TestDeleteCustomerManagedKeyNotInProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	// CMK belongs to a different project.
	helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, "other-project-id"))

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --organization-id %s --project-id %s --rw", testCMKID, testOrgID, testProjectID))

	helper.AssertErr(fmt.Sprintf("Error: could not find customer-managed-key %s in project %s", testCMKID, testProjectID))
}

// TestDeleteCustomerManagedKey_StdoutIsValidJSON is the CLI-82 regression-pin
// for the delete-success narration: pre-fix, stdout had "Operation Successful"
// instead of structured JSON. Reverting the Pattern B fmt.Fprintf/PrintBodyMap
// pair to cmd.Println causes this test to fail.
func TestDeleteCustomerManagedKey_StdoutIsValidJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))
	mockHandler.AddResponse(http.StatusNoContent, "")

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --organization-id %s --project-id %s --rw --format json", testCMKID, testOrgID, testProjectID))

	helper.AssertOutIsValidJSON()
}

func TestDeleteCustomerManagedKey_TableFormat(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))
	mockHandler.AddResponse(http.StatusNoContent, "")

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --organization-id %s --project-id %s --rw --format table", testCMKID, testOrgID, testProjectID))

	helper.AssertOutContainsStrings([]string{"DELETED", "ID", "true", testCMKID})
	helper.AssertErrContainsStrings([]string{fmt.Sprintf("customer-managed-key %s deleted", testCMKID)})
}

func TestDeleteCustomerManagedKeyError(t *testing.T) {
	testCases := []struct {
		statusCode    int
		expectedError string
		returnBody    string
	}{
		{
			statusCode:    http.StatusBadRequest,
			expectedError: fmt.Sprintf("Error: [Can not delete encryption key %s. The key is linked to an active instance.]", testCMKID),
			returnBody: fmt.Sprintf(`{
				"errors": [
				  {
					"message": "Can not delete encryption key %s. The key is linked to an active instance.",
					"reason": "encryption-key-is-active"
				  }
				]
			  }`, testCMKID),
		},
		{
			statusCode:    http.StatusNotFound,
			expectedError: fmt.Sprintf("Error: [Encryption Key not found: %s]", testCMKID),
			returnBody: fmt.Sprintf(`{
				"errors": [
				  {
					"message": "Encryption Key not found: %s",
					"reason": "encryption-key-not-found"
				  }
				]
			  }`, testCMKID),
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("StatusCode%d", testCase.statusCode), func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			registerProjectsMock(&helper)

			// First call GET (pre-flight OK), second call DELETE (error).
			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", testCMKID), http.StatusOK, cmkGetBody(testCMKID, testProjectID))
			mockHandler.AddResponse(testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --organization-id %s --project-id %s --rw", testCMKID, testOrgID, testProjectID))

			mockHandler.AssertCalledTimes(2)
			mockHandler.AssertCalledWithMethod(http.MethodDelete)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}
