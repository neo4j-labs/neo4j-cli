// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

// TestGetEvaluationRun covers the full record: JSON output carries the
// per-question results in addition to the run summary.
func TestGetEvaluationRun(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, fmt.Sprintf(`{
		"data": {
			"id": "%s",
			"agent_id": "%s",
			"dataset_id": "%s",
			"status": "succeeded",
			"metrics": ["accuracy"],
			"avg_accuracy": 1,
			"created_at": "2026-06-09T10:15:00Z",
			"started_at": "2026-06-09T10:15:05Z",
			"finished_at": "2026-06-09T10:16:00Z",
			"questions": [
				{
					"query_number": 1,
					"query_text": "Who is the CEO?",
					"expected_answer": "Jane",
					"metrics": [{"name": "accuracy", "value": 1}]
				}
			]
		}
	}`, testRunID, testAgentID, testDatasetID))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run get %s --organization-id=%s --project-id=%s", testRunID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"agent_id": "%s",
		"avg_accuracy": 1,
		"created_at": "2026-06-09T10:15:00Z",
		"dataset_id": "%s",
		"finished_at": "2026-06-09T10:16:00Z",
		"id": "%s",
		"metrics": [
			"accuracy"
		],
		"questions": [
			{
				"expected_answer": "Jane",
				"metrics": [
					{
						"name": "accuracy",
						"value": 1
					}
				],
				"query_number": 1,
				"query_text": "Who is the CEO?"
			}
		],
		"started_at": "2026-06-09T10:15:05Z",
		"status": "succeeded"
	}
}`, testAgentID, testDatasetID, testRunID))
}

// TestGetEvaluationRunWithOrganizationAndProjectIdFromConfig pins the scope
// precedence: flags win, the default workspace is the fallback.
func TestGetEvaluationRunWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, fmt.Sprintf(`{
		"data": {"id": "%s", "agent_id": "%s", "dataset_id": "%s", "status": "running", "created_at": "2026-06-09T10:15:00Z"}
	}`, testRunID, testAgentID, testDatasetID))

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run get %s --format json", testRunID))

	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{testRunID, "running"})
}

func TestGetEvaluationRunWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, fmt.Sprintf(`{
		"data": {
			"id": "%s",
			"status": "succeeded",
			"metrics": ["accuracy"],
			"avg_accuracy": 0.83,
			"created_at": "2026-06-09T10:15:00Z",
			"started_at": "2026-06-09T10:15:05Z",
			"finished_at": "2026-06-09T10:16:00Z"
		}
	}`, testRunID))

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run get %s --organization-id=%s --project-id=%s", testRunID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{
		"ID", "STATUS", "METRICS", "AVG_ACCURACY", "AVG_TOOL_TRAJECTORY", "AVG_GROUNDEDNESS", "AVG_RELEVANCE", "CREATED_AT", "STARTED_AT", "FINISHED_AT",
		testRunID, "succeeded", "0.83", "2026-06-09T10:15:00Z", "2026-06-09T10:15:05Z", "2026-06-09T10:16:00Z",
	})
}

func TestGetEvaluationRunNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath("non-existent-run-id"), http.StatusNotFound, `{
		"errors": [{"message": "Evaluation run not found"}]
	}`)

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run get non-existent-run-id --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	helper.AssertErr(`Error: [
	Evaluation run not found
]`)
}
