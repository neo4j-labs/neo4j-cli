// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListInstances(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{
			"data": [
				{
					"id": "2f49c2b3",
					"name": "Production",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				},
				{
					"id": "b51dc964",
					"name": "Instance01",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "aws"
				},
				{
					"id": "432392ae",
					"name": "Recommendations",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "azure"
				},
				{
					"id": "524b7d8d",
					"name": "Northwind",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				}
			]
		}`)

	helper.ExecuteCommand("instance list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	  "data": [
		{
		  "cloud_provider": "gcp",
		  "id": "2f49c2b3",
		  "name": "Production",
		  "tenant_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "aws",
		  "id": "b51dc964",
		  "name": "Instance01",
		  "tenant_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "azure",
		  "id": "432392ae",
		  "name": "Recommendations",
		  "tenant_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "gcp",
		  "id": "524b7d8d",
		  "name": "Northwind",
		  "tenant_id": "YOUR_TENANT_ID"
		}
	  ]
	}`)
}

func TestListInstancesWithTenantId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/instances", http.StatusOK, `{
			"data": [
				{
					"id": "2f49c2b3",
					"name": "Production",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				},
				{
					"id": "b51dc964",
					"name": "Instance01",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "aws"
				},
				{
					"id": "432392ae",
					"name": "Recommendations",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "azure"
				},
				{
					"id": "524b7d8d",
					"name": "Northwind",
					"tenant_id": "YOUR_TENANT_ID",
					"cloud_provider": "gcp"
				}
			]
		}`)

	helper.ExecuteCommand("instance list --tenant-id my-tenant-id")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("tenantId", "my-tenant-id")

	helper.AssertOutJson(`{
	  "data": [
		{
		  "cloud_provider": "gcp",
		  "id": "2f49c2b3",
		  "name": "Production",
		  "tenant_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "aws",
		  "id": "b51dc964",
		  "name": "Instance01",
		  "tenant_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "azure",
		  "id": "432392ae",
		  "name": "Recommendations",
		  "tenant_id": "YOUR_TENANT_ID"
		},
		{
		  "cloud_provider": "gcp",
		  "id": "524b7d8d",
		  "name": "Northwind",
		  "tenant_id": "YOUR_TENANT_ID"
		}
	  ]
	}`)
}

func TestListCustomerManagedKeysWithInvalidOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("instance list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}
