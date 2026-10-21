// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
	"github.com/stretchr/testify/require"
)

func TestListEvaluationDatasets(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{
		"data": [
			{
				"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				"dataset_name": "Product FAQ",
				"description": "Smoke tests",
				"question_count": 2,
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z"
			},
			{
				"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				"dataset_name": "Second dataset",
				"description": "",
				"question_count": 1,
				"created_at": "2026-02-01T00:00:00Z",
				"updated_at": "2026-02-01T00:00:00Z"
			}
		],
		"links": {"next": null}
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset list --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	"data": [
		{
			"created_at": "2026-01-01T00:00:00Z",
			"dataset_name": "Product FAQ",
			"description": "Smoke tests",
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"question_count": 2,
			"updated_at": "2026-01-02T00:00:00Z"
		},
		{
			"created_at": "2026-02-01T00:00:00Z",
			"dataset_name": "Second dataset",
			"description": "",
			"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"question_count": 1,
			"updated_at": "2026-02-01T00:00:00Z"
		}
	]
}`)
}

func TestListEvaluationDatasetsWithDefaultWorkspace(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{
		"data": [
			{
				"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				"dataset_name": "Product FAQ",
				"description": "Smoke tests",
				"question_count": 2,
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z"
			}
		],
		"links": {"next": null}
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand("agent evaluation-dataset list")

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(`{
	"data": [
		{
			"created_at": "2026-01-01T00:00:00Z",
			"dataset_name": "Product FAQ",
			"description": "Smoke tests",
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"question_count": 2,
			"updated_at": "2026-01-02T00:00:00Z"
		}
	]
}`)
}

// TestListEvaluationDatasetsLimitStopsEarly checks that --limit trims the result
// and that the truncation is announced on stderr — a short list that looks
// complete is the failure mode the walk-by-default contract exists to prevent.
func TestListEvaluationDatasetsLimitStopsEarly(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{
		"data": [
			{"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479", "dataset_name": "one", "question_count": 1},
			{"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "dataset_name": "two", "question_count": 1},
			{"id": "b2c3d4e5-f6a7-8901-bcde-f12345678901", "dataset_name": "three", "question_count": 1}
		],
		"links": {"next": "`+datasetsPath()+`?page_limit=1000&page_token=more"}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset list --limit 2 --organization-id=%s --project-id=%s --format json", testOrgID, testProjectID))

	// The limit is satisfied by the first page, so the second must not be fetched.
	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "a1b2c3d4-e5f6-7890-abcd-ef1234567890"})
	require.NotContains(t, helper.PrintOut(), "b2c3d4e5-f6a7-8901-bcde-f12345678901", "results beyond --limit must be trimmed")
	helper.AssertErrContainsStrings([]string{"Showing the first 2 evaluation datasets; more are available"})
}

// TestListEvaluationDatasetsFollowsAllPages checks the walk-by-default contract:
// a multi-page collection is followed to exhaustion and emitted as one merged
// envelope, with the cursor from each links.next fed into the next request.
func TestListEvaluationDatasetsFollowsAllPages(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{
		"data": [{"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479", "dataset_name": "one", "question_count": 1}],
		"links": {"next": "`+datasetsPath()+`?page_limit=1000&page_token=cursor-two"}
	}`)
	mockHandler.AddResponse(http.StatusOK, `{
		"data": [{"id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "dataset_name": "two", "question_count": 1}],
		"links": {"next": null}
	}`)

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset list --organization-id=%s --project-id=%s --format json", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(2)
	mockHandler.AssertCalledWithQueryParam("page_token", "cursor-two")
	helper.AssertOutContainsStrings([]string{"f47ac10b-58cc-4372-a567-0e02b2c3d479", "a1b2c3d4-e5f6-7890-abcd-ef1234567890"})
	helper.AsssertOk()
}

// TestListEvaluationDatasetsTableOutput pins the summary columns the table form
// renders.
func TestListEvaluationDatasetsTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{
		"data": [
			{
				"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				"dataset_name": "Product FAQ",
				"description": "Smoke tests",
				"question_count": 2,
				"created_at": "2026-01-01T00:00:00Z",
				"updated_at": "2026-01-02T00:00:00Z"
			}
		],
		"links": {"next": null}
	}`)

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset list --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{
		"ID", "DATASET_NAME", "DESCRIPTION", "QUESTION_COUNT", "CREATED_AT", "UPDATED_AT",
		"f47ac10b-58cc-4372-a567-0e02b2c3d479", "Product FAQ", "Smoke tests", "2", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z",
	})
}

func TestListEvaluationDatasetsWithMissingProjectId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset list --organization-id=%s", testOrgID))

	helper.AssertErr("Error: no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'")
}

func TestListEvaluationDatasetsWithMissingOrganizationId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset list --project-id=%s", testProjectID))

	helper.AssertErr("Error: no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'")
}
