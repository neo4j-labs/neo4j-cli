// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// questionsFs returns an in-memory filesystem holding one questions file.
func questionsFs(t *testing.T, contents string) afero.Fs {
	t.Helper()
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/questions.json", []byte(contents), 0o644))
	return fs
}

// requireUsageError asserts err is a usage error (exit code 2) whose message
// contains want.
func requireUsageError(t *testing.T, err error, want string) {
	t.Helper()
	var ce *clierr.CLIError
	require.Error(t, err)
	require.True(t, errors.As(err, &ce), "want *clierr.CLIError, got %T: %v", err, err)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, want)
}

func TestReadQuestionsFile_ConvertsEveryField(t *testing.T) {
	fs := questionsFs(t, `[
		{
			"query_number": 1,
			"query_text": "What is Neo4j?",
			"expected_answer": "A graph database",
			"expected_tool_calls": [
				{"tool_name": "query-tool", "tool_type": "text2cypher", "tool_input": {"query": "What is Neo4j?"}, "tool_output": {"rows": 1}}
			]
		},
		{
			"question_id": "3f1c2b1a-0000-4000-8000-000000000001",
			"query_number": 2,
			"query_text": "How do I query it?",
			"expected_answer": "With Cypher"
		}
	]`)

	questions, err := readQuestionsFile(fs, "/questions.json")
	require.NoError(t, err)
	require.Len(t, questions, 2)

	assert.Equal(t, 1, questions[0].QueryNumber)
	assert.Equal(t, "What is Neo4j?", questions[0].QueryText)
	assert.Equal(t, "A graph database", questions[0].ExpectedAnswer)
	assert.Empty(t, questions[0].QuestionID, "create must not send a question_id")
	require.Len(t, questions[0].ExpectedToolCalls, 1)
	assert.Equal(t, "query-tool", questions[0].ExpectedToolCalls[0].ToolName)
	assert.Equal(t, "text2cypher", questions[0].ExpectedToolCalls[0].ToolType)
	assert.Equal(t, map[string]any{"query": "What is Neo4j?"}, questions[0].ExpectedToolCalls[0].ToolInput)
	assert.Equal(t, map[string]any{"rows": float64(1)}, questions[0].ExpectedToolCalls[0].ToolOutput)

	assert.Equal(t, "3f1c2b1a-0000-4000-8000-000000000001", questions[1].QuestionID, "an update patch keeps the question_id")
	assert.Nil(t, questions[1].ExpectedToolCalls, "an absent expected_tool_calls stays nil so the body omits it")
}

func TestReadQuestionsFile_PresentEmptyToolCallsStayAnEmptyArray(t *testing.T) {
	fs := questionsFs(t, `[{"query_number": 1, "query_text": "q", "expected_answer": "a", "expected_tool_calls": []}]`)

	questions, err := readQuestionsFile(fs, "/questions.json")
	require.NoError(t, err)
	require.Len(t, questions, 1)
	assert.NotNil(t, questions[0].ExpectedToolCalls, "a present empty array must not collapse to nil")
	assert.Empty(t, questions[0].ExpectedToolCalls)
}

func TestReadQuestionsFile_FailuresAreUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents string
		want     string
	}{
		{
			name:     "invalid JSON",
			contents: "not-valid-json",
			want:     "expected a JSON array of questions",
		},
		{
			name:     "not an array",
			contents: `{"questions": []}`,
			want:     "expected a JSON array of questions",
		},
		{
			name:     "empty array",
			contents: `[]`,
			want:     "must contain between 1 and 50 questions, got 0",
		},
		{
			name:     "missing query_number",
			contents: `[{"query_text": "q", "expected_answer": "a"}]`,
			want:     "question 1",
		},
		{
			name:     "query_number below one",
			contents: `[{"query_number": 0, "query_text": "q", "expected_answer": "a"}]`,
			want:     "query_number must be 1 or greater",
		},
		{
			name:     "empty query_text",
			contents: `[{"query_number": 1, "query_text": "", "expected_answer": "a"}]`,
			want:     "query_text is required",
		},
		{
			name:     "empty expected_answer",
			contents: `[{"query_number": 1, "query_text": "q", "expected_answer": ""}]`,
			want:     "expected_answer is required",
		},
		{
			name:     "tool call without tool_name",
			contents: `[{"query_number": 1, "query_text": "q", "expected_answer": "a", "expected_tool_calls": [{"tool_type": "function"}]}]`,
			want:     "tool call 1: tool_name is required",
		},
		{
			name:     "tool call without tool_type",
			contents: `[{"query_number": 1, "query_text": "q", "expected_answer": "a", "expected_tool_calls": [{"tool_name": "t"}]}]`,
			want:     "tool call 1: tool_type is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readQuestionsFile(questionsFs(t, tc.contents), "/questions.json")

			requireUsageError(t, err, tc.want)
		})
	}
}

func TestReadQuestionsFile_RejectsMoreThanFiftyQuestions(t *testing.T) {
	questions := make([]string, 51)
	for i := range questions {
		questions[i] = fmt.Sprintf(`{"query_number": %d, "query_text": "q", "expected_answer": "a"}`, i+1)
	}

	_, err := readQuestionsFile(questionsFs(t, "["+strings.Join(questions, ",")+"]"), "/questions.json")

	requireUsageError(t, err, "must contain between 1 and 50 questions, got 51")
}

func TestReadQuestionsFile_MissingFileIsAUsageError(t *testing.T) {
	_, err := readQuestionsFile(afero.NewMemMapFs(), "/missing.json")

	requireUsageError(t, err, `could not read --questions-file file "/missing.json"`)
}
