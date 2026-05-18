// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package session_test

import (
	"net/http"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testOrgID     = "test-org-id"
	testProjectID = "YOUR_PROJECT_ID"
	testSessionID = "559c94c7-15de43fg"
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

// sessionGetBody returns a minimal GET /graph-analytics/sessions/{id} response
// body with the given tenant_id. Used to satisfy pre-flight ownership checks.
func sessionGetBody(id, tenantID string) string {
	return `{
		"data": {
			"id": "` + id + `",
			"name": "people-and-fruits",
			"memory": "8GB",
			"instance_id": null,
			"status": "Ready",
			"created_at": "2025-04-04T09:32:35Z",
			"host": "` + id + `.neo4j.io",
			"expiry_date": "2025-04-11T09:32:35Z",
			"ttl": "20m0s",
			"user_id": "YOUR_USER_ID",
			"tenant_id": "` + tenantID + `",
			"cloud_provider": "gcp",
			"region": "europe-west1"
		}
	}`
}
