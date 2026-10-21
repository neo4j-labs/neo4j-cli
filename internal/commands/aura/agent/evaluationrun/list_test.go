// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

func TestListEvaluationRuns(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusOK, `{
		"data": [
			{
				"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				"agent_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"dataset_id": "b1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"status": "succeeded",
				"metrics": ["accuracy"],
				"created_at": "2026-06-09T10:15:00Z"
			},
			{
				"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"agent_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"dataset_id": "b1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"status": "running",
				"metrics": ["accuracy", "groundedness"],
				"created_at": "2026-06-09T10:20:00Z"
			}
		],
		"links": {"next": null}
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	"data": [
		{
			"agent_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"created_at": "2026-06-09T10:15:00Z",
			"dataset_id": "b1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"metrics": [
				"accuracy"
			],
			"status": "succeeded"
		},
		{
			"agent_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"created_at": "2026-06-09T10:20:00Z",
			"dataset_id": "b1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"metrics": [
				"accuracy",
				"groundedness"
			],
			"status": "running"
		}
	]
}`)
}

// TestListEvaluationRunsWalksAllPages pins the default traversal: a first page
// with a links.next cursor must be followed so the output is the complete list,
// not the API's first page.
func TestListEvaluationRunsWalksAllPages(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	next := fmt.Sprintf("%s%s?page_token=cursor-two", helper.Server.URL, evalRunsPath())
	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusOK, fmt.Sprintf(`{
		"data": [
			{"id": "run-1", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:15:00Z"}
		],
		"links": {"next": %q}
	}`, testAgentID, testDatasetID, next))
	mockHandler.AddResponse(http.StatusOK, fmt.Sprintf(`{
		"data": [
			{"id": "run-2", "agent_id": "%s", "dataset_id": "%s", "status": "failed", "created_at": "2026-06-09T10:20:00Z"}
		],
		"links": {"next": null}
	}`, testAgentID, testDatasetID))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithQueryParam("page_token", "cursor-two")
	helper.AssertOutContainsStrings([]string{"run-1", "run-2"})
}

// TestListEvaluationRunsWithAgentFilter pins the --agent-id filter riding on
// the request; the service omits the parameter entirely when it is unset.
func TestListEvaluationRunsWithAgentFilter(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusOK, fmt.Sprintf(`{
		"data": [
			{"id": "run-1", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:15:00Z"}
		],
		"links": {"next": null}
	}`, testAgentID, testDatasetID))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --agent-id=%s --organization-id=%s --project-id=%s", testAgentID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithQueryParam("agent_id", testAgentID)
	helper.AssertOutContainsStrings([]string{"run-1"})
}

// TestListEvaluationRunsLimitStopsEarly checks that --limit trims the result
// and that the truncation is announced — a short list that looks complete is
// the failure mode the pagination note exists to prevent.
func TestListEvaluationRunsLimitStopsEarly(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	next := fmt.Sprintf("%s%s?page_token=cursor-two", helper.Server.URL, evalRunsPath())
	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusOK, fmt.Sprintf(`{
		"data": [
			{"id": "run-1", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:15:00Z"},
			{"id": "run-2", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:16:00Z"},
			{"id": "run-3", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:17:00Z"}
		],
		"links": {"next": %q}
	}`, testAgentID, testDatasetID, testAgentID, testDatasetID, testAgentID, testDatasetID, next))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --limit 2 --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	// The limit is satisfied by the first page, so the second must not be fetched.
	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{"run-1", "run-2"})
	helper.AssertErrContainsStrings([]string{"Showing the first 2 evaluation runs; more are available"})
}

// TestListEvaluationRunsLimitExactlyMatchesTotal guards the off-by-one: when
// the limit equals the collection size there is nothing more to fetch, so the
// truncation note must not fire.
func TestListEvaluationRunsLimitExactlyMatchesTotal(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(evalRunsPath(), http.StatusOK, fmt.Sprintf(`{
		"data": [
			{"id": "run-1", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:15:00Z"},
			{"id": "run-2", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:16:00Z"}
		],
		"links": {"next": null}
	}`, testAgentID, testDatasetID, testAgentID, testDatasetID))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --limit 2 --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	// AssertErr("") pins that no truncation note was written.
	helper.AsssertOk()
}

func TestListEvaluationRunsWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusOK, fmt.Sprintf(`{
		"data": [
			{"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "created_at": "2026-06-09T10:15:00Z"}
		],
		"links": {"next": null}
	}`, testAgentID, testDatasetID))

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{"ID", "AGENT_ID", "DATASET_ID", "STATUS", "CREATED_AT", "f47ac10b-58cc-4372-a567-0e02b2c3d479", "succeeded", "2026-06-09T10:15:00Z"})
}

func TestListEvaluationRunsWithMissingProjectId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --organization-id=%s", testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestListEvaluationRunsWithMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run list --project-id=%s", testProjectID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}
