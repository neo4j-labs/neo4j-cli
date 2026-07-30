// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"fmt"
	"net/http"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

const (
	testOrgID          = "test-org-id"
	testProjectID      = "YOUR_PROJECT_ID"
	testVirtualGraphID = "ge82059a"
)

// registerProjectsMock registers a mock for the v2beta1 list-projects endpoint
// that returns testProjectID as a valid project, satisfying the org/project
// scope validation every virtual-graph command performs up front.
func registerProjectsMock(helper *testutils.AuraTestHelper) {
	helper.NewRequestHandlerMock(
		"/v2beta1/organizations/"+testOrgID+"/projects",
		http.StatusOK,
		`{"data": [{"id": "`+testProjectID+`", "name": "Test Project"}]}`,
	)
}

// virtualGraphsPath is the v2beta1 org/project-scoped collection path.
func virtualGraphsPath() string {
	return fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/virtual-graphs", testOrgID, testProjectID)
}

// virtualGraphPath is the v2beta1 org/project-scoped path for a single virtual
// graph.
func virtualGraphPath(virtualGraphID string) string {
	return fmt.Sprintf("%s/%s", virtualGraphsPath(), virtualGraphID)
}

// virtualGraphBody returns a single-resource GET/PATCH response body in the
// public API's VirtualGraph shape, with the lowercase status the API returns.
func virtualGraphBody(id, status string) string {
	return `{
		"data": {
			"id": "` + id + `",
			"name": "sales-analytics",
			"status": "` + status + `",
			"cloud_provider": "gcp",
			"region": "europe-west1",
			"memory": "4Gi",
			"bolt_url": "neo4j+s://` + id + `.graph-engine.neo4j.io",
			"data_source_id": "ds-abc123",
			"data_source_type": "databricks-pat",
			"error_detail": "",
			"created_at": "2026-06-09T10:15:00Z"
		}
	}`
}
