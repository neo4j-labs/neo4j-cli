// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

func TestGetEvaluationDataset(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, `{
		"data": {
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"dataset_name": "Product FAQ",
			"description": "Smoke tests",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-02T00:00:00Z",
			"questions": [
				{
					"question_id": "3f1c2b1a-0000-4000-8000-000000000001",
					"query_number": 1,
					"query_text": "What is Neo4j?",
					"expected_answer": "A graph database",
					"expected_tool_calls": [{"tool_name": "query-tool", "tool_type": "text2cypher"}]
				}
			]
		}
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset get %s --organization-id=%s --project-id=%s", testDatasetID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)

	// The detail record is the full server record: the questions array survives
	// into the JSON output.
	helper.AssertOutJson(`{
	"data": {
		"created_at": "2026-01-01T00:00:00Z",
		"dataset_name": "Product FAQ",
		"description": "Smoke tests",
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
		"questions": [
			{
				"expected_answer": "A graph database",
				"expected_tool_calls": [
					{
						"tool_name": "query-tool",
						"tool_type": "text2cypher"
					}
				],
				"query_number": 1,
				"query_text": "What is Neo4j?",
				"question_id": "3f1c2b1a-0000-4000-8000-000000000001"
			}
		],
		"updated_at": "2026-01-02T00:00:00Z"
	}
}`)
}

// TestGetEvaluationDatasetTableOutput pins the summary columns the table form
// renders for a detail record.
func TestGetEvaluationDatasetTableOutput(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, `{
		"data": {
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"dataset_name": "Product FAQ",
			"description": "Smoke tests",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-02T00:00:00Z",
			"questions": [{"query_number": 1, "query_text": "q", "expected_answer": "a"}]
		}
	}`)

	helper.SetConfigValue("format", "table")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset get %s --organization-id=%s --project-id=%s", testDatasetID, testOrgID, testProjectID))

	mockHandler.AssertCalledTimes(1)
	helper.AssertOutContainsStrings([]string{
		"ID", "DATASET_NAME", "DESCRIPTION", "QUESTION_COUNT", "CREATED_AT", "UPDATED_AT",
		"f47ac10b-58cc-4372-a567-0e02b2c3d479", "Product FAQ", "Smoke tests", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z",
	})
}

func TestGetEvaluationDatasetWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.SetDefaultProjectInConfig(testOrgID, testProjectID)

	mockHandler := helper.NewRequestHandlerMock(datasetPath(testDatasetID), http.StatusOK, `{
		"data": {
			"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"dataset_name": "Product FAQ",
			"description": "Smoke tests",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-02T00:00:00Z",
			"questions": []
		}
	}`)

	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset get %s", testDatasetID))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
}

func TestGetEvaluationDatasetWithInvalidId(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	helper.ExecuteCommand(fmt.Sprintf("agent evaluation-dataset get ../../x --organization-id=%s --project-id=%s", testOrgID, testProjectID))

	helper.AssertErr(`Error: invalid evaluation-dataset id "../../x"`)
}
