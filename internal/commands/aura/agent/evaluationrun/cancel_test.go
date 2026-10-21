// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

func TestCancelEvaluationRun(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID)+"/cancel", http.StatusOK, fmt.Sprintf(`{
		"data": {
			"id": "%s",
			"agent_id": "%s",
			"dataset_id": "%s",
			"status": "cancelled",
			"metrics": ["accuracy"],
			"created_at": "2026-06-09T10:15:00Z"
		}
	}`, testRunID, testAgentID, testDatasetID))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run cancel %s --organization-id=%s --project-id=%s --rw", testRunID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"agent_id": "%s",
		"created_at": "2026-06-09T10:15:00Z",
		"dataset_id": "%s",
		"id": "%s",
		"metrics": [
			"accuracy"
		],
		"status": "cancelled"
	}
}`, testAgentID, testDatasetID, testRunID))
}

func TestCancelEvaluationRunWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath(testRunID)+"/cancel", http.StatusOK, fmt.Sprintf(`{
		"data": {"id": "%s", "status": "cancelled"}
	}`, testRunID))

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run cancel %s --rw --format json", testRunID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertOutContainsStrings([]string{testRunID, "cancelled"})
}

func TestCancelEvaluationRunNotFound(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunPath("non-existent-run-id")+"/cancel", http.StatusNotFound, `{
		"errors": [{"message": "Evaluation run not found"}]
	}`)

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-run cancel non-existent-run-id --organization-id=%s --project-id=%s --rw", testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)

	helper.AssertErr(`Error: [
	Evaluation run not found
]`)
}
