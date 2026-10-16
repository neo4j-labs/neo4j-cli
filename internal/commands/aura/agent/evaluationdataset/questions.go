// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset

import (
	"encoding/json"
	"fmt"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/afero"
)

// questionsFileFlag is the flag naming the JSON questions file. It is shared by
// create and update so the flag name and the errors that mention it are one
// string.
const questionsFileFlag = "questions-file"

// The API accepts between 1 and 50 questions in a dataset (minItems/maxItems on
// the create and update payloads).
const (
	minQuestions = 1
	maxQuestions = 50
)

// questionFile is the JSON shape of one item in a --questions-file array. The
// service's EvaluationQuestion carries no JSON tags, so the file is decoded
// through this local wire struct instead. Pointer fields distinguish a missing
// required key from a present zero value.
type questionFile struct {
	QueryNumber       *int           `json:"query_number"`
	QueryText         *string        `json:"query_text"`
	ExpectedAnswer    *string        `json:"expected_answer"`
	ExpectedToolCalls []toolCallFile `json:"expected_tool_calls"`
	QuestionID        string         `json:"question_id"`
}

// toolCallFile is the JSON shape of one expected tool call inside a question.
type toolCallFile struct {
	ToolName   *string        `json:"tool_name"`
	ToolType   *string        `json:"tool_type"`
	ToolInput  map[string]any `json:"tool_input"`
	ToolOutput any            `json:"tool_output"`
}

// readQuestionsFile reads the JSON array at path through fs and converts it to
// the service's question type, validating it against the API's constraints
// (1-50 questions, each with query_number, query_text and expected_answer)
// before any request is made. Every failure is a usage error.
func readQuestionsFile(fs afero.Fs, path string) ([]auraclient.EvaluationQuestion, error) {
	contents, err := afero.ReadFile(fs, path)
	if err != nil {
		return nil, clierr.NewUsageError("could not read --%s file %q: %s", questionsFileFlag, path, err.Error())
	}

	var items []questionFile
	if err := json.Unmarshal(contents, &items); err != nil {
		return nil, clierr.NewUsageError("invalid --%s file %q: expected a JSON array of questions: %s", questionsFileFlag, path, err.Error())
	}
	if len(items) < minQuestions || len(items) > maxQuestions {
		return nil, clierr.NewUsageError("--%s file %q must contain between %d and %d questions, got %d", questionsFileFlag, path, minQuestions, maxQuestions, len(items))
	}

	questions := make([]auraclient.EvaluationQuestion, 0, len(items))
	for i, item := range items {
		question, err := item.toQuestion(path, i+1)
		if err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}

	return questions, nil
}

// toQuestion converts one decoded file item into the service type. position is
// the item's 1-based position in the file, used to point at the offending
// question in validation errors.
func (q questionFile) toQuestion(path string, position int) (auraclient.EvaluationQuestion, error) {
	if q.QueryNumber == nil {
		return auraclient.EvaluationQuestion{}, questionError(path, position, "query_number is required")
	}
	if *q.QueryNumber < 1 {
		return auraclient.EvaluationQuestion{}, questionError(path, position, "query_number must be 1 or greater")
	}
	if q.QueryText == nil || *q.QueryText == "" {
		return auraclient.EvaluationQuestion{}, questionError(path, position, "query_text is required")
	}
	if q.ExpectedAnswer == nil || *q.ExpectedAnswer == "" {
		return auraclient.EvaluationQuestion{}, questionError(path, position, "expected_answer is required")
	}

	question := auraclient.EvaluationQuestion{
		QueryNumber:    *q.QueryNumber,
		QueryText:      *q.QueryText,
		ExpectedAnswer: *q.ExpectedAnswer,
		QuestionID:     q.QuestionID,
	}

	// An absent expected_tool_calls stays nil so the request body omits it; an
	// empty array is sent as an empty array.
	if q.ExpectedToolCalls != nil {
		question.ExpectedToolCalls = make([]auraclient.EvaluationToolCall, 0, len(q.ExpectedToolCalls))
		for i, call := range q.ExpectedToolCalls {
			if call.ToolName == nil || *call.ToolName == "" {
				return auraclient.EvaluationQuestion{}, questionError(path, position, fmt.Sprintf("tool call %d: tool_name is required", i+1))
			}
			if call.ToolType == nil || *call.ToolType == "" {
				return auraclient.EvaluationQuestion{}, questionError(path, position, fmt.Sprintf("tool call %d: tool_type is required", i+1))
			}
			question.ExpectedToolCalls = append(question.ExpectedToolCalls, auraclient.EvaluationToolCall{
				ToolName:   *call.ToolName,
				ToolType:   *call.ToolType,
				ToolInput:  call.ToolInput,
				ToolOutput: call.ToolOutput,
			})
		}
	}

	return question, nil
}

// questionError builds the usage error for one invalid question.
func questionError(path string, position int, detail string) error {
	return clierr.NewUsageError("invalid question %d in --%s file %q: %s", position, questionsFileFlag, path, detail)
}
