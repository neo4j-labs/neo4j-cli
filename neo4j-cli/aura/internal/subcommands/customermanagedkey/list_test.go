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

		helper.ExecuteCommand(fmt.Sprintf("%s list", command))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)

		helper.AssertOutJson(`{
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
		}
		`)
	}
}

func TestListCustomerManagedKeysWithTenantId(t *testing.T) {
	for _, command := range []string{"customer-managed-key", "cmk"} {
		helper := testutils.NewAuraTestHelper(t)
		defer helper.Close()

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

		helper.ExecuteCommand(fmt.Sprintf("%s list --tenant-id 1234", command))

		mockHandler.AssertCalledTimes(1)
		mockHandler.AssertCalledWithMethod(http.MethodGet)
		mockHandler.AssertCalledWithQueryParam("tenantId", "1234")

		helper.AssertOutJson(`{
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
		}
		`)
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
