// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package customermanagedkey_test

import (
	"net/http"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testOrgID     = "test-org-id"
	testProjectID = "YOUR_TENANT_ID"
	testCMKID     = "8c764aed-8eb3-4a1c-92f6-e4ef0c7a6ed9"
)

// registerProjectsMock registers a mock for the v2beta1 list-projects endpoint
// that returns testProjectID as a valid project.
func registerProjectsMock(helper *testutils.AuraTestHelper) {
	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": [{"id": "`+testProjectID+`", "name": "Test Project"}]}`,
	)
}

// cmkGetBody returns a minimal GET /customer-managed-keys/{id} response body
// with the given tenant_id. Used to satisfy pre-flight ownership checks in
// mutating command tests.
func cmkGetBody(id, tenantID string) string {
	return `{
		"data": {
			"id": "` + id + `",
			"name": "Instance01",
			"created": "2024-01-31T14:06:57Z",
			"cloud_provider": "aws",
			"key_id": "arn:aws:kms:us-east-1:123456789:key/11111-a222-1212-x789-1212f1212f",
			"region": "us-east-1",
			"type": "enterprise-db",
			"tenant_id": "` + tenantID + `",
			"status": "ready"
		}
	}`
}
