// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

// listBody returns a list response with the given links.next value (a raw JSON
// literal: either `null` or a quoted absolute URL).
func listBody(next string) string {
	return `{
		"data": [
			{
				"id": "ge82059a",
				"name": "sales-analytics",
				"status": "running",
				"cloud_provider": "gcp",
				"region": "europe-west1",
				"memory": "4Gi",
				"bolt_url": "neo4j+s://ge82059a.graph-engine.neo4j.io",
				"data_source_id": "ds-abc123",
				"data_source_type": "databricks-pat",
				"error_detail": "",
				"created_at": "2026-06-09T10:15:00Z"
			}
		],
		"links": {
			"self": "https://api.neo4j.io/v2beta1/organizations/o/projects/p/virtual-graphs",
			"first": "https://api.neo4j.io/v2beta1/organizations/o/projects/p/virtual-graphs",
			"next": ` + next + `
		}
	}`
}

func TestListVirtualGraphs(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody("null"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
  "data": [
    {
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
  ]
}`)
}

func TestListVirtualGraphsWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody("null"))

	helper.ExecuteCommand("virtual-graph list")

	mockHandler.AssertCalledTimes(1)
	helper.AsssertOk()
}

// TestListVirtualGraphsLastPageEmitsNoHint verifies that a null links.next
// produces no stderr narration, so a single-page listing stays quiet.
func TestListVirtualGraphsLastPageEmitsNoHint(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody("null"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AsssertOk()
}

// TestListVirtualGraphsPagination covers both halves of the cursor contract:
// --page-limit / --page-token reach the API as query params, and the cursor
// embedded in links.next is surfaced to stderr rather than silently dropped
// with the rest of the links envelope.
func TestListVirtualGraphsPagination(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	next := `"https://api.neo4j.io/v2beta1/organizations/o/projects/p/virtual-graphs?page_limit=1&page_token=cursor-two"`
	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody(next))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --page-limit 1 --page-token cursor-one --organization-id %s --project-id %s", testOrgID, testProjectID))

	mockHandler.AssertCalledWithQueryParam("page_limit", "1")
	mockHandler.AssertCalledWithQueryParam("page_token", "cursor-one")

	helper.AssertErrContainsStrings([]string{"More results available. Re-run with --page-token cursor-two"})
}

// TestListVirtualGraphsPaginationRelativeLink covers links.next arriving as a
// relative URL rather than the absolute one the API examples show. Both forms
// occur in practice, and the cursor must be extracted either way.
func TestListVirtualGraphsPaginationRelativeLink(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody(`"?page_limit=1&page_token=cursor-two"`))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --page-limit 1 --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{"More results available. Re-run with --page-token cursor-two"})
}

// TestListVirtualGraphsOmitsUnsetPaginationParams guards against sending
// page_limit=0, which would ask the API for an empty page.
func TestListVirtualGraphsOmitsUnsetPaginationParams(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody("null"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s", testOrgID, testProjectID))

	for _, call := range mockHandler.Calls {
		if call.QueryParams.Has("page_limit") || call.QueryParams.Has("page_token") {
			t.Fatalf("pagination params must be omitted when unset; got %v", call.QueryParams)
		}
	}
}

func TestListVirtualGraphsMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("virtual-graph list")

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}
