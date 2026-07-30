// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/require"
)

// vgRow renders one virtual graph list row.
func vgRow(id string) string {
	return `{
		"id": "` + id + `",
		"name": "sales-analytics",
		"status": "running",
		"cloud_provider": "gcp",
		"region": "europe-west1",
		"memory": "4Gi",
		"bolt_url": "neo4j+s://` + id + `.graph-engine.neo4j.io",
		"data_source_id": "ds-abc123",
		"data_source_type": "databricks-pat",
		"error_detail": "",
		"created_at": "2026-06-09T10:15:00Z"
	}`
}

// listPage builds a list response holding the given ids, with next as a raw
// JSON literal (`null`, or a quoted absolute/relative URL).
func listPage(next string, ids ...string) string {
	rows := ""
	for i, id := range ids {
		if i > 0 {
			rows += ","
		}
		rows += vgRow(id)
	}
	return `{
		"data": [` + rows + `],
		"links": {
			"self": "https://api.neo4j.io/v2beta1/organizations/o/projects/p/virtual-graphs",
			"first": "https://api.neo4j.io/v2beta1/organizations/o/projects/p/virtual-graphs",
			"next": ` + next + `
		}
	}`
}

// listBody is the single-page shape used by the non-pagination tests.
func listBody(next string) string {
	return listPage(next, "ge82059a")
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

// TestListVirtualGraphsRequestsMaxPageSize pins the wire contract: the CLI asks
// for the largest page the API allows so a large collection costs few round
// trips, and sends no cursor on the first request.
func TestListVirtualGraphsRequestsMaxPageSize(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listBody("null"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s", testOrgID, testProjectID))

	mockHandler.AssertCalledWithQueryParam("page_limit", fmt.Sprintf("%d", api.ListPageSize))
	require.False(t, mockHandler.Calls[0].QueryParams.Has("page_token"), "first request must not carry a cursor")
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

// TestListVirtualGraphsFollowsAllPages is the core of the auto-follow contract:
// a multi-page collection is walked to exhaustion and emitted as one merged
// envelope, with the cursor from each links.next fed into the next request.
func TestListVirtualGraphsFollowsAllPages(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listPage(`"?page_token=cursor-two"`, "ge00001", "ge00002"))
	mockHandler.AddResponse(http.StatusOK, listPage(`"?page_token=cursor-three"`, "ge00003"))
	mockHandler.AddResponse(http.StatusOK, listPage("null", "ge00004"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s --format json", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(3)
	mockHandler.AssertCalledWithQueryParam("page_token", "cursor-two")
	mockHandler.AssertCalledWithQueryParam("page_token", "cursor-three")

	helper.AssertOutContainsStrings([]string{"ge00001", "ge00002", "ge00003", "ge00004"})
	// Following to the end means there is nothing left to warn about.
	helper.AsssertOk()
}

// TestListVirtualGraphsFollowsRelativeNextLink covers links.next arriving as a
// relative URL rather than the absolute one the API examples show. Both forms
// occur in practice, and the cursor must be followed either way.
func TestListVirtualGraphsFollowsRelativeNextLink(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listPage(`"?page_limit=1000&page_token=cursor-two"`, "ge00001"))
	mockHandler.AddResponse(http.StatusOK, listPage("null", "ge00002"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s --format json", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithQueryParam("page_token", "cursor-two")
	helper.AssertOutContainsStrings([]string{"ge00001", "ge00002"})
}

// TestListVirtualGraphsLimitStopsEarly checks that --limit trims the result and
// that the truncation is announced — a short list that looks complete is the
// failure mode this whole rework exists to prevent.
func TestListVirtualGraphsLimitStopsEarly(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listPage(`"?page_token=cursor-two"`, "ge00001", "ge00002", "ge00003"))
	mockHandler.AddResponse(http.StatusOK, listPage("null", "ge00004"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --limit 2 --organization-id %s --project-id %s --format json", testOrgID, testProjectID))

	// The limit is satisfied by the first page, so the second must not be fetched.
	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{"ge00001", "ge00002"})
	require.NotContains(t, helper.PrintOut(), "ge00003", "results beyond --limit must be trimmed")
	helper.AssertErrContainsStrings([]string{"Showing the first 2 virtual graphs; more are available"})
}

// TestListVirtualGraphsLimitExactlyMatchesTotal guards the off-by-one: when the
// limit happens to equal the collection size there is nothing more to fetch, so
// the "more are available" notice must not fire.
func TestListVirtualGraphsLimitExactlyMatchesTotal(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, listPage("null", "ge00001", "ge00002"))

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --limit 2 --organization-id %s --project-id %s --format json", testOrgID, testProjectID))

	helper.AsssertOk()
	require.NotContains(t, helper.PrintErr(), "more are available")
}

// TestListVirtualGraphsNonAdvancingCursorErrors covers the loop guard: the
// cursor is opaque to the CLI, so a server handing back a token it already
// issued must produce an error rather than an infinite request loop.
func TestListVirtualGraphsNonAdvancingCursorErrors(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	stuck := listPage(`"?page_token=cursor-stuck"`, "ge00001")
	mockHandler := helper.NewRequestHandlerMock(virtualGraphsPath(), http.StatusOK, stuck)
	mockHandler.AddResponse(http.StatusOK, stuck)
	mockHandler.AddResponse(http.StatusOK, stuck)

	err := helper.ExecuteCommandE(fmt.Sprintf("virtual-graph list --organization-id %s --project-id %s", testOrgID, testProjectID))

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
	require.Contains(t, ce.Message, "already returned")
	// First request plus the one repeat that detects the stall — not a loop.
	mockHandler.AssertCalledTimes(2)
}

func TestListVirtualGraphsRejectsNegativeLimit(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	registerProjectsMock(&helper)

	helper.ExecuteCommand(fmt.Sprintf("virtual-graph list --limit -1 --organization-id %s --project-id %s", testOrgID, testProjectID))

	helper.AssertErrContainsStrings([]string{"--limit must be zero or greater"})
}

func TestListVirtualGraphsMissingOrg(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand("virtual-graph list")

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}
