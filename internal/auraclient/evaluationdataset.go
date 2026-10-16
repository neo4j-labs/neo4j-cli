// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// EvaluationDataset is an Aura Agent evaluation dataset. Record is the full
// server record, which is what commands render: a list row is the API's
// summary (id, dataset_name, description, created_at, updated_at,
// question_count), and Get's is the detail, its questions array included.
type EvaluationDataset struct {
	ID            string
	Name          string
	Description   string
	QuestionCount int

	Record map[string]any
}

// EvaluationToolCall is a tool call a question expects the agent to make.
// ToolInput and ToolOutput are sent only when set.
type EvaluationToolCall struct {
	ToolName   string
	ToolType   string
	ToolInput  map[string]any
	ToolOutput any
}

// EvaluationQuestion is one question in an evaluation dataset. QuestionID
// identifies a question that already exists and is sent only by Update; Create
// never sends it, because the API rejects it there.
type EvaluationQuestion struct {
	QueryNumber       int
	QueryText         string
	ExpectedAnswer    string
	ExpectedToolCalls []EvaluationToolCall

	QuestionID string
}

// EvaluationDatasetSpec is the complete definition used to create a dataset.
// The API requires a name and at least one question.
type EvaluationDatasetSpec struct {
	Name        string
	Description string
	Questions   []EvaluationQuestion
}

// EvaluationDatasetPatch is a partial update: only non-nil fields are sent, and
// at least one must be set because the API rejects an empty patch. Questions is
// sent when non-nil (a question item may carry its QuestionID to update it).
type EvaluationDatasetPatch struct {
	Name        *string
	Description *string
	Questions   []EvaluationQuestion
}

// EvaluationDatasetPage is one traversal of the list endpoint.
type EvaluationDatasetPage struct {
	Items []EvaluationDataset
	// LimitReached reports that traversal stopped at the caller's limit while
	// more items were available.
	LimitReached bool
	// PageCapReached reports that traversal stopped at the page cap, so the
	// list may be incomplete.
	PageCapReached bool
}

// EvaluationDatasetService operates on Aura Agent evaluation datasets, which
// are project-scoped.
type EvaluationDatasetService interface {
	// List walks every page, stopping after limit items when limit > 0.
	List(ctx context.Context, scope Scope, limit int) (*EvaluationDatasetPage, error)
	Get(ctx context.Context, scope Scope, id string) (*EvaluationDataset, error)
	// Create sends the dataset's name, description and questions. The API's
	// response carries only the new dataset's id; the record holds just that.
	Create(ctx context.Context, scope Scope, spec EvaluationDatasetSpec) (*EvaluationDataset, error)
	// Update applies the patch and returns the dataset detail the API sent
	// back.
	Update(ctx context.Context, scope Scope, id string, patch EvaluationDatasetPatch) (*EvaluationDataset, error)
	Delete(ctx context.Context, scope Scope, id string) error
}

type evaluationDatasetService struct {
	cfg *clicfg.Config
}

func newEvaluationDataset(rec map[string]any) EvaluationDataset {
	d := EvaluationDataset{
		ID:          str(rec, "id"),
		Name:        str(rec, "dataset_name"),
		Description: str(rec, "description"),
		Record:      rec,
	}
	// A list row is a summary and carries question_count; a detail record
	// carries the questions array instead.
	if n, ok := rec["question_count"].(float64); ok {
		d.QuestionCount = int(n)
	} else if questions, ok := rec["questions"].([]any); ok {
		d.QuestionCount = len(questions)
	}
	return d
}

func (s evaluationDatasetService) path(scope Scope, id string) (string, error) {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return "", err
	}
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/organizations/%s/projects/%s/agents/evaluation-datasets", scope.OrgID, scope.ProjectID)
	if id != "" {
		if err := ValidateResourceID("evaluation-dataset", id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	return p, nil
}

// one sends a request whose response carries one dataset record in the
// {"data": ...} envelope, and must come back with status 200.
func (s evaluationDatasetService) one(ctx context.Context, method, path string, body map[string]any, doing string) (*EvaluationDataset, error) {
	rows, err := v2Rows(ctx, s.cfg, method, path, body, nil, doing, http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, doing)
	if err != nil {
		return nil, err
	}
	d := newEvaluationDataset(rec)
	return &d, nil
}

// List walks the cursor-paginated collection (page_limit/page_token) from the
// first page to the last, following each response's links.next. The endpoint
// caps one response well below the size of a large collection, so a single GET
// would silently return a prefix.
func (s evaluationDatasetService) List(ctx context.Context, scope Scope, limit int) (*EvaluationDatasetPage, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, clierr.NewUsageError("--limit must be zero or greater; zero returns every evaluation dataset")
	}
	res, err := transport.ListAllPages(ctx, s.cfg, p, transport.AuraApiVersion2, limit)
	if err != nil {
		return nil, err
	}
	page := &EvaluationDatasetPage{LimitReached: res.LimitReached, PageCapReached: res.PageCapReached}
	for _, it := range res.Items {
		page.Items = append(page.Items, newEvaluationDataset(it))
	}
	return page, nil
}

func (s evaluationDatasetService) Get(ctx context.Context, scope Scope, id string) (*EvaluationDataset, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodGet, p, nil, "fetching evaluation dataset")
}

func (s evaluationDatasetService) Create(ctx context.Context, scope Scope, spec EvaluationDatasetSpec) (*EvaluationDataset, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodPost, p, spec.body(), "creating evaluation dataset")
}

func (s evaluationDatasetService) Update(ctx context.Context, scope Scope, id string, patch EvaluationDatasetPatch) (*EvaluationDataset, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	body := patch.body()
	if len(body) == 0 {
		return nil, clierr.NewUsageError("an evaluation dataset update must change at least one of name, description or questions")
	}
	return s.one(ctx, http.MethodPatch, p, body, "updating evaluation dataset")
}

func (s evaluationDatasetService) Delete(ctx context.Context, scope Scope, id string) error {
	p, err := s.path(scope, id)
	if err != nil {
		return err
	}
	// The response is a bare empty object, not a {"data": ...} envelope, so
	// there is nothing to decode; only the status matters.
	_, status, err := transport.MakeRequest(ctx, s.cfg, p, &transport.RequestConfig{
		Method:  http.MethodDelete,
		Version: transport.AuraApiVersion2,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected status %d deleting evaluation dataset", status)
	}
	return nil
}

func (spec EvaluationDatasetSpec) body() map[string]any {
	return map[string]any{
		"dataset_name": spec.Name,
		"description":  spec.Description,
		"questions":    questionBodies(spec.Questions, false),
	}
}

func (p EvaluationDatasetPatch) body() map[string]any {
	body := map[string]any{}
	if p.Name != nil {
		body["dataset_name"] = *p.Name
	}
	if p.Description != nil {
		body["description"] = *p.Description
	}
	if p.Questions != nil {
		body["questions"] = questionBodies(p.Questions, true)
	}
	return body
}

// questionBodies renders question items for a create (withQuestionID false) or
// a patch (true) body.
func questionBodies(questions []EvaluationQuestion, withQuestionID bool) []map[string]any {
	out := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		out = append(out, q.body(withQuestionID))
	}
	return out
}

func (q EvaluationQuestion) body(withQuestionID bool) map[string]any {
	item := map[string]any{
		"query_number":    q.QueryNumber,
		"query_text":      q.QueryText,
		"expected_answer": q.ExpectedAnswer,
	}
	if withQuestionID && q.QuestionID != "" {
		item["question_id"] = q.QuestionID
	}
	if q.ExpectedToolCalls != nil {
		calls := make([]map[string]any, 0, len(q.ExpectedToolCalls))
		for _, c := range q.ExpectedToolCalls {
			calls = append(calls, c.body())
		}
		item["expected_tool_calls"] = calls
	}
	return item
}

func (c EvaluationToolCall) body() map[string]any {
	call := map[string]any{
		"tool_name": c.ToolName,
		"tool_type": c.ToolType,
	}
	if c.ToolInput != nil {
		call["tool_input"] = c.ToolInput
	}
	if c.ToolOutput != nil {
		call["tool_output"] = c.ToolOutput
	}
	return call
}
