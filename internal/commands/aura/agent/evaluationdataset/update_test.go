// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

// updateResponseBody is the detail record the API returns from a PATCH.
const updateResponseBody = `{
	"data": {
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"dataset_name": "Renamed dataset",
		"description": "Smoke tests",
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-02T00:00:00Z",
		"questions": [
			{"question_id": "3f1c2b1a-0000-4000-8000-000000000001", "query_number": 1, "query_text": "What is Neo4j?", "expected_answer": "A graph database"}
		]
	}
}`

func TestUpdateEvaluationDatasetNameOnly(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, updateResponseBody)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset update %s --name "Renamed dataset" --organization-id %s --project-id %s --rw`,
		testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	// Only the set field is sent; the unset name/description/questions keys are absent.
	mockHandler.AssertCalledWithBody(`{"dataset_name": "Renamed dataset"}`)

	helper.AssertOutJson(`{
	"data": {
		"created_at": "2026-01-01T00:00:00Z",
		"dataset_name": "Renamed dataset",
		"description": "Smoke tests",
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"questions": [
			{
				"expected_answer": "A graph database",
				"query_number": 1,
				"query_text": "What is Neo4j?",
				"question_id": "3f1c2b1a-0000-4000-8000-000000000001"
			}
		],
		"updated_at": "2026-01-02T00:00:00Z"
	}
}`)
}

func TestUpdateEvaluationDatasetDescription(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, updateResponseBody)

	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset update %s --description "Updated description" --organization-id %s --project-id %s --rw`,
		testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	mockHandler.AssertCalledWithBody(`{"description": "Updated description"}`)
}

// TestUpdateEvaluationDatasetQuestionsFile checks that a questions patch reads
// the file through cfg.Fs() and keeps the question_id, which tells the API the
// question already exists.
func TestUpdateEvaluationDatasetQuestionsFile(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, updateResponseBody)

	helper.SeedFile("/questions.json", `[
		{
			"question_id": "3f1c2b1a-0000-4000-8000-000000000001",
			"query_number": 1,
			"query_text": "What is Neo4j?",
			"expected_answer": "A graph database",
			"expected_tool_calls": [{"tool_name": "query-tool", "tool_type": "text2cypher", "tool_input": {"query": "What is Neo4j?"}}]
		}
	]`)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset update %s --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testDatasetID, testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPatch)
	mockHandler.AssertCalledWithBody(`{
		"questions": [
			{
				"question_id": "3f1c2b1a-0000-4000-8000-000000000001",
				"query_number": 1,
				"query_text": "What is Neo4j?",
				"expected_answer": "A graph database",
				"expected_tool_calls": [{"tool_name": "query-tool", "tool_type": "text2cypher", "tool_input": {"query": "What is Neo4j?"}}]
			}
		]
	}`)
}

func TestUpdateEvaluationDatasetWithNoFieldsSet(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, updateResponseBody)

	helper.ExecuteCommand(fmt.Sprintf(
		"agent evaluation-dataset update %s --organization-id %s --project-id %s --rw",
		testDatasetID, testOrgID, testProjectID,
	))

	helper.AssertErr("Error: at least one of --name, --description or --questions-file must be set")
	mockHandler.AssertCalledTimes(0)
}

func TestUpdateEvaluationDatasetWithInvalidQuestionsFile(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, updateResponseBody)

	helper.SeedFile("/questions.json", `[{"query_number": 0, "query_text": "q", "expected_answer": "a"}]`)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset update %s --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testDatasetID, testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: invalid question 1 in --questions-file file "/questions.json": query_number must be 1 or greater`)
	mockHandler.AssertCalledTimes(0)
}

func TestUpdateEvaluationDatasetWithUnreadableQuestionsFile(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, updateResponseBody)

	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset update %s --questions-file /missing.json --organization-id %s --project-id %s --rw`,
		testDatasetID, testOrgID, testProjectID,
	))

	helper.AssertErrContainsStrings([]string{`could not read --questions-file file "/missing.json"`})
	mockHandler.AssertCalledTimes(0)
}
