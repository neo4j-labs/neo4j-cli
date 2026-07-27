// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/require"
)

func TestGetVirtualGraph(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusOK, virtualGraphBody(testVirtualGraphID, "running"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph get %s --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
  "data": {
    "bolt_url": "neo4j+s://ge82059a.graph-engine.neo4j.io",
    "cloud_provider": "gcp",
    "created_at": "2026-06-09T10:15:00Z",
    "data_source_id": "ds-abc123",
    "data_source_type": "databricks-pat",
    "error_detail": "",
    "id": "ge82059a",
    "memory": "4Gi",
    "name": "sales-analytics",
    "region": "europe-west1",
    "status": "running"
  }
}`)
}

func TestGetVirtualGraphWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusOK, virtualGraphBody(testVirtualGraphID, "running"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph get %s", testVirtualGraphID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AsssertOk()
}

// TestGetVirtualGraphIncludesMaximumBytesBilled covers the BigQuery-only
// maximum_bytes_billed field, which is appended to the projection only when the
// API returns it.
func TestGetVirtualGraphIncludesMaximumBytesBilled(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusOK, `{
		"data": {
			"id": "ge82059a",
			"name": "bq-analytics",
			"status": "running",
			"cloud_provider": "gcp",
			"region": "europe-west1",
			"memory": "4Gi",
			"bolt_url": "neo4j+s://ge82059a.graph-engine.neo4j.io",
			"data_source_id": "ds-bq001",
			"data_source_type": "bigquery",
			"error_detail": "",
			"created_at": "2026-06-09T10:15:00Z",
			"maximum_bytes_billed": 1099511627776
		}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph get %s --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	helper.AssertOutContainsStrings([]string{`"maximum_bytes_billed": 1099511627776`})
}

// TestGetVirtualGraphNotFound verifies that a 404 on the v2beta1 scoped
// virtual-graph path surfaces as a not-found error tagged with resource type
// "virtual-graph" by the API layer's parseResourceFromRequest (which resolves
// the trailing plural/id pair of the nested path).
func TestGetVirtualGraphNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(virtualGraphPath(testVirtualGraphID), http.StatusNotFound, `{
		"errors": [
			{"message": "virtual graph not found", "reason": "not-found"}
		]
	}`)

	err := helper.ExecuteCommandE(fmt.Sprintf("virtual-graph get %s --organization-id %s --project-id %s", testVirtualGraphID, testOrgID, testProjectID))

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Equal(t, 3, ce.Code)
	require.Equal(t, "virtual-graph", ce.ResourceType)
	require.Equal(t, testVirtualGraphID, ce.ResourceID)
	require.Equal(t, "Run 'neo4j-cli aura virtual-graph list --project-id <id>' to see virtual graphs in this project.", ce.Suggestion)
}

func TestGetVirtualGraphMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph get %s", testVirtualGraphID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}

func TestGetVirtualGraphMissingProject(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph get %s --organization-id %s", testVirtualGraphID, testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

// TestGetVirtualGraphRejectsTraversalID covers the ValidateResourceID guard: a
// ".." id would otherwise be resolved away by url.JoinPath and silently
// retarget the request at the collection path.
func TestGetVirtualGraphRejectsTraversalID(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph get .. --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErr(`Error: invalid virtual-graph id ".."`)
}
