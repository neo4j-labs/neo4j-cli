// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tenant_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListTenants(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/tenants", http.StatusOK, `{
			"data": [
				{
				"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
				"name": "Production"
				},
				{
				"id": "YOUR_TENANT_ID",
				"name": "Staging"
				},
				{
				"id": "da045ab3-3b89-4f45-8b96-528f2e47cd13",
				"name": "Development"
				}
			]
		}`)

	helper.ExecuteCommand("tenant list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
		"data": [
			{
				"id": "6981ace7-efe8-4f5c-b7c5-267b5162ce91",
				"name": "Production"
			},
			{
				"id": "YOUR_TENANT_ID",
				"name": "Staging"
			},
			{
				"id": "da045ab3-3b89-4f45-8b96-528f2e47cd13",
				"name": "Development"
			}
		]
	}
	`)
}

func TestListCustomerManagedKeysWithInvalidOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("tenant list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}

func TestListTenantsWithCredentialFlag(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
	}{
		{name: "--credential flag", command: "tenant list --credential named-cred"},
		{name: "-c shorthand", command: "tenant list -c named-cred"},
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

			mockHandler := helper.NewRequestHandlerMock("/v1/tenants", http.StatusOK, `{"data": []}`)

			helper.ExecuteCommand(tc.command)

			mockHandler.AssertCalledTimes(1)
			helper.AssertErrContainsStrings([]string{"Warning: 'aura tenant list' is deprecated and will be removed in a future release. Use 'aura project list' instead."})
		})
	}
}

func TestListTenants_DeprecationWarning(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock("/v1/tenants", http.StatusOK, `{"data": []}`)

	helper.ExecuteCommand("tenant list")

	helper.AssertErrContainsStrings([]string{"Warning: 'aura tenant list' is deprecated and will be removed in a future release. Use 'aura project list' instead."})
}
