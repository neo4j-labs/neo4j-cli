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

		cmkId := "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9"

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", cmkId), http.StatusOK, `{
				"data": {
					"id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
					"name": "Instance01",
					"created": "2024-01-31T14:06:57Z",
					"cloud_provider": "aws",
					"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
					"region": "us-east-1",
					"type": "enterprise-db",
					"tenant_id": "YOUR_TENANT_ID",
					"status": "ready"
				}
			}`)

		helper.ExecuteCommand(fmt.Sprintf("%s get %s", command, cmkId))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)

		helper.AssertOutJson(`{
		  "data": {
			"cloud_provider": "aws",
			"created": "2024-01-31T14:06:57Z",
			"id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
			"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
			"name": "Instance01",
			"region": "us-east-1",
			"status": "ready",
			"tenant_id": "YOUR_TENANT_ID",
			"type": "enterprise-db"
		  }
		}`)
	}
}

func TestGetCustomerManagedKeyWithTrailingNewline(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	cmkId := "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9"

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", cmkId), http.StatusOK, `{
			"data": {
				"id": "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9",
				"name": "Instance01",
				"created": "2024-01-31T14:06:57Z",
				"cloud_provider": "aws",
				"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
				"region": "us-east-1",
				"type": "enterprise-db",
				"tenant_id": "YOUR_TENANT_ID",
				"status": "ready"
			}
		}`)

	helper.ExecuteCommand(fmt.Sprintf("customer-managed-key get %s\"\n\"", cmkId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
}

func TestGetCustomerManagedKeyNotFoundError(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

		cmkId := "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9"

		mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v1/customer-managed-keys/%s", cmkId), http.StatusNotFound, fmt.Sprintf(`{
			"errors": [
				{
				"message": "Encryption Key not found: %s",
				"reason": "encryption-key-not-found"
				}
			]
			}`, cmkId))

		helper.ExecuteCommand(fmt.Sprintf("%s get %s", command, cmkId))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)

		helper.AssertErr(fmt.Sprintf("Error: [Encryption Key not found: %s]", cmkId))
	}
}
