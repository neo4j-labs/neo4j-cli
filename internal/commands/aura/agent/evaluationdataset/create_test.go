// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/commands/aura/testutils"
)

// testQuestionsFile is the JSON array a --questions-file test seeds into the
// in-memory filesystem the command reads through cfg.Fs().
const testQuestionsFile = `[
	{
		"query_number": 1,
		"query_text": "What is Neo4j?",
		"expected_answer": "A graph database",
		"expected_tool_calls": [{"tool_name": "query-tool", "tool_type": "text2cypher"}]
	},
	{
		"query_number": 2,
		"query_text": "How do I query it?",
		"expected_answer": "With Cypher"
	}
]`

func TestCreateEvaluationDataset(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{
		"data": {"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479"},
		"links": {"resource": "`+datasetsPath()+`/f47ac10b-58cc-4372-a567-0e02b2c3d479"}
	}`)

	helper.SeedFile("/questions.json", testQuestionsFile)
	helper.SetConfigValue("format", "json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --description "Smoke tests" --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{
		"dataset_name": "Product FAQ",
		"description": "Smoke tests",
		"questions": [
			{
				"query_number": 1,
				"query_text": "What is Neo4j?",
				"expected_answer": "A graph database",
				"expected_tool_calls": [{"tool_name": "query-tool", "tool_type": "text2cypher"}]
			},
			{
				"query_number": 2,
				"query_text": "How do I query it?",
				"expected_answer": "With Cypher"
			}
		]
	}`)

	helper.AssertOutJson(`{
	"data": {
		"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	}
}`)
}

func TestCreateEvaluationDatasetWithoutDescription(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "f47ac10b-58cc-4372-a567-0e02b2c3d479"}}`)

	helper.SeedFile("/questions.json", testQuestionsFile)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodPost)
	mockHandler.AssertCalledWithBody(`{
		"dataset_name": "Product FAQ",
		"description": "",
		"questions": [
			{
				"query_number": 1,
				"query_text": "What is Neo4j?",
				"expected_answer": "A graph database",
				"expected_tool_calls": [{"tool_name": "query-tool", "tool_type": "text2cypher"}]
			},
			{
				"query_number": 2,
				"query_text": "How do I query it?",
				"expected_answer": "With Cypher"
			}
		]
	}`)
}

func TestCreateEvaluationDatasetWithMissingName(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

	helper.SeedFile("/questions.json", testQuestionsFile)
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: required flag(s) "name" not set`)
	mockHandler.AssertCalledTimes(0)
}

func TestCreateEvaluationDatasetWithMissingQuestionsFileFlag(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: required flag(s) "questions-file" not set`)
	mockHandler.AssertCalledTimes(0)
}

// TestCreateEvaluationDatasetWithUnreadableQuestionsFile proves the file is read
// through cfg.Fs(): a path that was never seeded into the in-memory filesystem
// fails as a usage error before any request.
func TestCreateEvaluationDatasetWithUnreadableQuestionsFile(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --questions-file /missing.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	helper.AssertErrContainsStrings([]string{`could not read --questions-file file "/missing.json"`})
	mockHandler.AssertCalledTimes(0)
}

func TestCreateEvaluationDatasetWithInvalidQuestionsJSON(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

	helper.SeedFile("/questions.json", "not-valid-json")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: invalid --questions-file file "/questions.json": expected a JSON array of questions: invalid character 'o' in literal null (expecting 'u')`)
	mockHandler.AssertCalledTimes(0)
}

func TestCreateEvaluationDatasetWithMissingQuestionField(t *testing.T) {
	for _, tc := range []struct {
		name     string
		question string
		want     string
	}{
		{
			name:     "missing query_number",
			question: `{"query_text": "q", "expected_answer": "a"}`,
			want:     "query_number is required",
		},
		{
			name:     "missing query_text",
			question: `{"query_number": 1, "expected_answer": "a"}`,
			want:     "query_text is required",
		},
		{
			name:     "missing expected_answer",
			question: `{"query_number": 1, "query_text": "q"}`,
			want:     "expected_answer is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

			helper.SeedFile("/questions.json", `[`+tc.question+`]`)
			helper.ExecuteCommand(fmt.Sprintf(
				`agent evaluation-dataset create --name "Product FAQ" --questions-file /questions.json --organization-id %s --project-id %s --rw`,
				testOrgID, testProjectID,
			))

			helper.AssertErr(`Error: invalid question 1 in --questions-file file "/questions.json": ` + tc.want)
			mockHandler.AssertCalledTimes(0)
		})
	}
}

func TestCreateEvaluationDatasetWithTooManyQuestions(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

	questions := make([]string, 51)
	for i := range questions {
		questions[i] = fmt.Sprintf(`{"query_number": %d, "query_text": "q", "expected_answer": "a"}`, i+1)
	}
	helper.SeedFile("/questions.json", "["+strings.Join(questions, ",")+"]")

	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: --questions-file file "/questions.json" must contain between 1 and 50 questions, got 51`)
	mockHandler.AssertCalledTimes(0)
}

func TestCreateEvaluationDatasetWithEmptyQuestionsFile(t *testing.T) {
	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(datasetsPath(), http.StatusOK, `{"data": {"id": "never"}}`)

	helper.SeedFile("/questions.json", "[]")
	helper.ExecuteCommand(fmt.Sprintf(
		`agent evaluation-dataset create --name "Product FAQ" --questions-file /questions.json --organization-id %s --project-id %s --rw`,
		testOrgID, testProjectID,
	))

	helper.AssertErr(`Error: --questions-file file "/questions.json" must contain between 1 and 50 questions, got 0`)
	mockHandler.AssertCalledTimes(0)
}
