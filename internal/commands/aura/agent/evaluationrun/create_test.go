// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

// queuedRunBody is a create response: the run was accepted but has not
// finished, so it carries no aggregate scores yet.
func queuedRunBody() string {
	return fmt.Sprintf(`{
		"id": "%s",
		"agent_id": "%s",
		"dataset_id": "%s",
		"status": "queued",
		"metrics": ["accuracy"],
		"created_at": "2026-06-09T10:15:00Z"
	}`, testRunID, testAgentID, testDatasetID)
}

func TestCreateEvaluationRun(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusAccepted, fmt.Sprintf(`{"data": %s}`, queuedRunBody()))

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --metric accuracy --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{
		"agent_id": "%s",
		"dataset_id": "%s",
		"metrics": ["accuracy"]
	}`, testAgentID, testDatasetID))

	helper.AssertOutJson(fmt.Sprintf(`{
	"data": {
		"agent_id": "%s",
		"created_at": "2026-06-09T10:15:00Z",
		"dataset_id": "%s",
		"id": "%s",
		"metrics": [
			"accuracy"
		],
		"status": "queued"
	}
}`, testAgentID, testDatasetID, testRunID))
}

// TestCreateEvaluationRunWithSeveralMetrics pins the repeatable --metric flag:
// every value must reach the API in order.
func TestCreateEvaluationRunWithSeveralMetrics(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusAccepted, fmt.Sprintf(`{"data": %s}`, queuedRunBody()))

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --metric accuracy --metric tool_trajectory --metric groundedness --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithBody(fmt.Sprintf(`{
		"agent_id": "%s",
		"dataset_id": "%s",
		"metrics": ["accuracy", "tool_trajectory", "groundedness"]
	}`, testAgentID, testDatasetID))
}

// TestCreateEvaluationRunWithWait covers the --wait path: the create call is
// accepted with status queued, the command polls until the status leaves
// queued/running, and the printed record is the re-fetched terminal one.
func TestCreateEvaluationRunWithWait(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	createMock := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusAccepted, fmt.Sprintf(`{"data": %s}`, queuedRunBody()))

	runningBody := fmt.Sprintf(`{"data": {"id": "%s", "agent_id": "%s", "dataset_id": "%s", "status": "running", "metrics": ["accuracy"], "created_at": "2026-06-09T10:15:00Z"}}`, testRunID, testAgentID, testDatasetID)
	succeededBody := fmt.Sprintf(`{"data": {"id": "%s", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "metrics": ["accuracy"], "created_at": "2026-06-09T10:15:00Z", "finished_at": "2026-06-09T10:16:00Z"}}`, testRunID, testAgentID, testDatasetID)
	finalBody := fmt.Sprintf(`{"data": {"id": "%s", "agent_id": "%s", "dataset_id": "%s", "status": "succeeded", "metrics": ["accuracy"], "avg_accuracy": 1, "created_at": "2026-06-09T10:15:00Z", "finished_at": "2026-06-09T10:16:00Z"}}`, testRunID, testAgentID, testDatasetID)

	pollMock := helper.NewRequestHandlerMock(evalRunPath(testRunID), http.StatusOK, runningBody)
	pollMock.AddResponse(http.StatusOK, succeededBody)
	// The final re-fetch: only this response carries the aggregate score, so
	// the assertion below proves the printed record came from Get, not from
	// the last poll.
	pollMock.AddResponse(http.StatusOK, finalBody)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --metric accuracy --wait --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	createMock.AssertCalledTimes(1)
	pollMock.AssertCalledTimes(3)

	helper.AssertErrContainsStrings([]string{
		fmt.Sprintf("Waiting for evaluation run %s to finish...", testRunID),
		"Evaluation Run Status: succeeded",
	})

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
		"status": "succeeded"
	}
}`, testAgentID, testDatasetID, testRunID))
}

// TestCreateEvaluationRunWithWaitRefusesMissingID pins the wait guard: a
// create response without an id would make the poller target the collection
// path, so the command must fail instead of polling the wrong resource.
func TestCreateEvaluationRunWithWaitRefusesMissingID(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.NewRequestHandlerMock(evalRunsPath(), http.StatusAccepted, fmt.Sprintf(`{"data": {"agent_id": "%s", "dataset_id": "%s", "status": "queued"}}`, testAgentID, testDatasetID))

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --metric accuracy --wait --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	helper.AssertErr("Error: create response did not carry an evaluation run id, cannot wait")
}

func TestCreateEvaluationRunRejectsUnknownMetric(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --metric bleu --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: invalid metric "bleu": must be one of: accuracy, tool_trajectory, groundedness, relevance`)
}

func TestCreateEvaluationRunRequiresAtLeastOneMetric(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	helper.AssertErr("Error: at least one --metric is required; pass --metric with one of: accuracy, tool_trajectory, groundedness, relevance")
}

func TestCreateEvaluationRunRequiresAgentAndDataset(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --metric accuracy --organization-id=%s --project-id=%s --rw",
		testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: required flag(s) "agent-id", "dataset-id" not set`)
}

func TestCreateEvaluationRunWithTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(evalRunsPath(), http.StatusAccepted, fmt.Sprintf(`{"data": %s}`, queuedRunBody()))

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-run create --agent-id=%s --dataset-id=%s --metric accuracy --organization-id=%s --project-id=%s --rw",
		testAgentID, testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{"ID", "AGENT_ID", "DATASET_ID", "STATUS", "CREATED_AT", testRunID, "queued", "2026-06-09T10:15:00Z"})
}
