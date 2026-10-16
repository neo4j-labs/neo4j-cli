// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// Evaluation run statuses while the run is still executing; WaitWhileRunning
// returns once the run leaves them.
const (
	EvaluationRunStatusQueued  = "queued"
	EvaluationRunStatusRunning = "running"
)

// EvaluationRun is an Aura Agent evaluation run. Record is the full server
// record, which is what commands render; the record Get returns additionally
// carries the run's per-question results ("questions").
type EvaluationRun struct {
	ID        string
	AgentID   string
	DatasetID string
	Status    string

	Record map[string]any
}

// EvaluationRunSpec describes an evaluation run to create. Metrics names the
// metrics to evaluate (accuracy, tool_trajectory, groundedness, relevance);
// agent_id, dataset_id and metrics are all required by the API.
type EvaluationRunSpec struct {
	AgentID   string
	DatasetID string
	Metrics   []string
}

// EvaluationRunPage is one traversal of the list endpoint.
type EvaluationRunPage struct {
	Items []EvaluationRun
	// LimitReached reports that traversal stopped at the caller's limit while
	// more items were available.
	LimitReached bool
	// PageCapReached reports that traversal stopped at the page cap, so the
	// list may be incomplete.
	PageCapReached bool
}

// EvaluationRunService operates on Aura Agent evaluation runs, which are
// project-scoped.
type EvaluationRunService interface {
	// List returns the project's evaluation runs, optionally only those of
	// agentID. It walks every page, stopping after limit items when limit > 0.
	List(ctx context.Context, scope Scope, agentID string, limit int) (*EvaluationRunPage, error)
	// Get returns one run; Record carries the run's per-question results.
	Get(ctx context.Context, scope Scope, id string) (*EvaluationRun, error)
	// Create starts a run and returns the accepted record (status queued or
	// running); use WaitWhileRunning to block until it finishes.
	Create(ctx context.Context, scope Scope, spec EvaluationRunSpec) (*EvaluationRun, error)
	// Delete deletes a run.
	Delete(ctx context.Context, scope Scope, id string) error
	// Cancel cancels a queued or running run.
	Cancel(ctx context.Context, scope Scope, id string) (*EvaluationRun, error)
	// WaitWhileRunning polls until the run's status is no longer queued or
	// running (case-insensitive) and returns the status it settled on.
	WaitWhileRunning(ctx context.Context, scope Scope, id string) (string, error)
}

type evaluationRunService struct {
	cfg *clicfg.Config
}

func newEvaluationRun(rec map[string]any) EvaluationRun {
	return EvaluationRun{
		ID:        str(rec, "id"),
		AgentID:   str(rec, "agent_id"),
		DatasetID: str(rec, "dataset_id"),
		Status:    str(rec, "status"),
		Record:    rec,
	}
}

// path builds the v2beta1 scoped path for the evaluation-runs collection, a
// single run, or a run sub-resource (e.g. cancel).
func (s evaluationRunService) path(scope Scope, id string, suffix ...string) (string, error) {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return "", err
	}
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/organizations/%s/projects/%s/agents/evaluation-runs", scope.OrgID, scope.ProjectID)
	if id != "" {
		if err := ValidateResourceID("evaluation-run", id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	for _, sfx := range suffix {
		p += "/" + sfx
	}
	return p, nil
}

func (s evaluationRunService) List(ctx context.Context, scope Scope, agentID string, limit int) (*EvaluationRunPage, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	if agentID != "" {
		if err := ValidateResourceID("agent", agentID); err != nil {
			return nil, err
		}
	}
	if limit < 0 {
		return nil, clierr.NewUsageError("--limit must be zero or greater; zero returns every evaluation run")
	}

	// The agent filter must ride on every page of the walk; an empty filter
	// sends no agent_id parameter at all.
	var extraQuery map[string]string
	if agentID != "" {
		extraQuery = map[string]string{"agent_id": agentID}
	}
	res, err := transport.ListAllPagesWithQuery(ctx, s.cfg, p, transport.AuraApiVersion2, limit, extraQuery)
	if err != nil {
		return nil, err
	}

	page := &EvaluationRunPage{LimitReached: res.LimitReached, PageCapReached: res.PageCapReached}
	for _, it := range res.Items {
		page.Items = append(page.Items, newEvaluationRun(it))
	}
	return page, nil
}

func (s evaluationRunService) Get(ctx context.Context, scope Scope, id string) (*EvaluationRun, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodGet, p, nil, nil, "fetching evaluation run", http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "fetching evaluation run")
	if err != nil {
		return nil, err
	}
	run := newEvaluationRun(rec)
	return &run, nil
}

func (s evaluationRunService) Create(ctx context.Context, scope Scope, spec EvaluationRunSpec) (*EvaluationRun, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodPost, p, spec.body(), nil, "creating evaluation run", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	run := newEvaluationRun(rows[0])
	return &run, nil
}

func (s evaluationRunService) Delete(ctx context.Context, scope Scope, id string) error {
	p, err := s.path(scope, id)
	if err != nil {
		return err
	}
	_, status, err := transport.MakeRequest(ctx, s.cfg, p, &transport.RequestConfig{
		Method:  http.MethodDelete,
		Version: transport.AuraApiVersion2,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected status %d deleting evaluation run", status)
	}
	return nil
}

func (s evaluationRunService) Cancel(ctx context.Context, scope Scope, id string) (*EvaluationRun, error) {
	p, err := s.path(scope, id, "cancel")
	if err != nil {
		return nil, err
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodPost, p, nil, nil, "cancelling evaluation run", http.StatusOK)
	if err != nil {
		return nil, err
	}
	run := newEvaluationRun(rows[0])
	return &run, nil
}

func (s evaluationRunService) WaitWhileRunning(ctx context.Context, scope Scope, id string) (string, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return "", err
	}
	// The comparison is case-insensitive on purpose: the status casing
	// returned by the API is not guaranteed, and a case-sensitive compare
	// would stop waiting on the first poll.
	resp, err := transport.PollWithVersion(ctx, s.cfg, p, transport.AuraApiVersion2, func(status string) bool {
		return !strings.EqualFold(status, EvaluationRunStatusQueued) && !strings.EqualFold(status, EvaluationRunStatusRunning)
	})
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}

// body is the create request payload; every field is required by the API.
func (spec EvaluationRunSpec) body() map[string]any {
	return map[string]any{
		"agent_id":   spec.AgentID,
		"dataset_id": spec.DatasetID,
		"metrics":    spec.Metrics,
	}
}
