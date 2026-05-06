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

			cmkId := "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9"

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", cmkId), http.StatusNoContent, "")

			helper.ExecuteCommand(fmt.Sprintf("%s delete %s --rw", command, cmkId))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodDelete)

			helper.AssertOut("Operation Successful\n")
		})
	}
}

func TestDeleteCustomerManagedKeyError(t *testing.T) {
	cmkId := "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9"

	testCases := []struct {
		statusCode    int
		expectedError string
		returnBody    string
	}{
		{
			statusCode:    http.StatusBadRequest,
			expectedError: fmt.Sprintf("Error: [Can not delete encryption key %s. The key is linked to an active instance.]", cmkId),
			returnBody: fmt.Sprintf(`{
				"errors": [
				  {
					"message": "Can not delete encryption key %s. The key is linked to an active instance.",
					"reason": "encryption-key-is-active"
				  }
				]
			  }`, cmkId),
		},
		{
			statusCode:    http.StatusNotFound,
			expectedError: fmt.Sprintf("Error: [Encryption Key not found: %s]", cmkId),
			returnBody: fmt.Sprintf(`{
				"errors": [
				  {
					"message": "Encryption Key not found: %s",
					"reason": "encryption-key-not-found"
				  }
				]
			  }`, cmkId),
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("StatusCode%d", testCase.statusCode), func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", cmkId), testCase.statusCode, testCase.returnBody)

			helper.ExecuteCommand(fmt.Sprintf("customer-managed-key delete %s --rw", cmkId))

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodDelete)

			helper.AssertOut("")
			helper.AssertErr(testCase.expectedError)
		})
	}
}
