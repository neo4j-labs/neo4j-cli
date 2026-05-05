// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestListSessions(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/graph-analytics/sessions", http.StatusOK, `{ "data": [
						{
						  "id": "s-04de43fe-67ab-4",
						  "name": "people-and-fruits",
						  "memory": "8GB",
						  "instance_id": null,
						  "status": "Ready",
						  "created_at": "2025-04-04T09:32:35Z",
						  "host": "s-04de43fe-67ab-4-gds.ORCHESTRA.neo4j.io",
						  "expiry_date": "2025-04-11T09:32:35Z",
						  "ttl": "20m0s",
						  "user_id": "YOUR_USER_ID",
						  "tenant_id": "YOUR_PROJECT_ID",
						  "cloud_provider": "azure",
						  "region": "francecentral"
						},
						{
						  "id": "559c94c7-15de43fg",
						  "name": "people-and-fruits-with-db",
						  "memory": "4GB",
						  "instance_id": "559c94c7",
						  "status": "Creating",
						  "created_at": "2025-04-04T09:32:35Z",
						  "host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
						  "expiry_date": null,
						  "ttl": null,
						  "user_id": "YOUR_USER_ID",
						  "tenant_id": "YOUR_PROJECT_ID",
						  "cloud_provider": "gcp",
						  "region": "europe-west1"
						}
				]
			}`)

	helper.ExecuteCommand("graph-analytics session list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	"data": [
		{
			"cloud_provider": "azure",
			"created_at": "2025-04-04T09:32:35Z",
			"expiry_date": "2025-04-11T09:32:35Z",
			"host": "s-04de43fe-67ab-4-gds.ORCHESTRA.neo4j.io",
			"id": "s-04de43fe-67ab-4",
			"instance_id": null,
			"memory": "8GB",
			"name": "people-and-fruits",
			"region": "francecentral",
			"status": "Ready",
			"tenant_id": "YOUR_PROJECT_ID",
			"ttl": "20m0s",
			"user_id": "YOUR_USER_ID"
		},
		{
			"cloud_provider": "gcp",
			"created_at": "2025-04-04T09:32:35Z",
			"expiry_date": null,
			"host": "559c94c7-15de43fg.ORCHESTRA.neo4j.io",
			"id": "559c94c7-15de43fg",
			"instance_id": "559c94c7",
			"memory": "4GB",
			"name": "people-and-fruits-with-db",
			"region": "europe-west1",
			"status": "Creating",
			"tenant_id": "YOUR_PROJECT_ID",
			"ttl": null,
			"user_id": "YOUR_USER_ID"
		}
	]
}`)
}

func TestListSessionsWithFilters(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock("/v1/graph-analytics/sessions", http.StatusOK, `{
			"data": []
		}`)

	helper.ExecuteCommand("graph-analytics session list --tenant-id my-tenant-id --organization-id my-org-id --instance-id my-instance-id")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("tenantId", "my-tenant-id")
	mockHandler.AssertCalledWithQueryParam("organizationId", "my-org-id")
	mockHandler.AssertCalledWithQueryParam("instanceId", "my-instance-id")

	helper.AssertOutJson(`{
	  "data": []
	}`)
}

func TestListCustomerManagedKeysWithInvalidOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("graph-analytics session list --format invalid")

	helper.AssertErr("Error: invalid format value specified: invalid")
}
