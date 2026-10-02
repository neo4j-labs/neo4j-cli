// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/clicfg"
)

// SessionStatusReady is the status of a graph analytics session that can be
// used.
const SessionStatusReady = api.GraphAnalyticsSessionReady

// Session is a Graph Analytics session. Record is the full server record after
// v2beta1 normalisation, which is what commands render.
type Session struct {
	ID        string
	Name      string
	Status    string
	ProjectID string

	Record map[string]any
}

// SessionCreate describes a session to create. Either InstanceID, or both
// CloudProvider and Region, identify where it runs.
type SessionCreate struct {
	Name          string
	Memory        string
	TTL           string // optional
	InstanceID    string // optional
	CloudProvider string // optional
	Region        string // optional
}

// SessionService operates on Graph Analytics sessions, which are
// project-scoped.
type SessionService interface {
	// List returns the project's sessions, optionally only those attached to
	// instanceID.
	List(ctx context.Context, scope Scope, instanceID string) ([]Session, error)
	Get(ctx context.Context, scope Scope, id string) (*Session, error)
	Create(ctx context.Context, scope Scope, spec SessionCreate) (*Session, error)
	// Delete returns the record the API sent back, or nil when it sent none.
	Delete(ctx context.Context, scope Scope, id string) (map[string]any, error)
	// WaitUntilReady polls until the session has left its creating state and
	// returns the status it settled on.
	WaitUntilReady(ctx context.Context, scope Scope, id string) (string, error)
}

type sessionService struct {
	cfg *clicfg.Config
}

func newSession(raw map[string]any) Session {
	rec := normalizeV2Beta1(raw)
	return Session{ID: str(rec, "id"), Name: str(rec, "name"), Status: str(rec, "status"), ProjectID: str(rec, "project_id"), Record: rec}
}

func (s sessionService) check(scope Scope, id string) error {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return err
	}
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return err
	}
	if id != "" {
		return ValidateResourceID("session", id)
	}
	return nil
}

func (s sessionService) List(ctx context.Context, scope Scope, instanceID string) ([]Session, error) {
	if err := s.check(scope, ""); err != nil {
		return nil, err
	}
	var query map[string]string
	if instanceID != "" {
		query = map[string]string{"instanceId": instanceID}
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodGet, api.ScopedSessionsPath(scope.OrgID, scope.ProjectID), nil, query, "listing sessions", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]Session, len(rows))
	for i, r := range rows {
		out[i] = newSession(r)
	}
	return out, nil
}

func (s sessionService) Get(ctx context.Context, scope Scope, id string) (*Session, error) {
	if err := s.check(scope, id); err != nil {
		return nil, err
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodGet, api.ScopedSessionPath(scope.OrgID, scope.ProjectID, id), nil, nil, "fetching session", http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "fetching session")
	if err != nil {
		return nil, err
	}
	sess := newSession(rec)
	return &sess, nil
}

func (s sessionService) Create(ctx context.Context, scope Scope, spec SessionCreate) (*Session, error) {
	if err := s.check(scope, ""); err != nil {
		return nil, err
	}
	body := map[string]any{
		"name":      spec.Name,
		"memory":    spec.Memory,
		"tenant_id": scope.ProjectID,
	}
	if spec.TTL != "" {
		body["ttl"] = spec.TTL
	}
	if spec.InstanceID != "" {
		body["instance_id"] = spec.InstanceID
	}
	if spec.CloudProvider != "" {
		body["cloud_provider"] = spec.CloudProvider
	}
	if spec.Region != "" {
		body["region"] = spec.Region
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodPost, api.ScopedSessionsPath(scope.OrgID, scope.ProjectID), body, nil, "creating session", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	sess := newSession(rows[0])
	return &sess, nil
}

func (s sessionService) Delete(ctx context.Context, scope Scope, id string) (map[string]any, error) {
	if err := s.check(scope, id); err != nil {
		return nil, err
	}
	body, status, err := api.MakeRequest(ctx, s.cfg, api.ScopedSessionPath(scope.OrgID, scope.ProjectID, id), &api.RequestConfig{
		Method:  http.MethodDelete,
		Version: api.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d deleting session", status)
	}
	if len(body) == 0 {
		return nil, nil
	}
	rows, err := decodeRows(body)
	if err != nil {
		return nil, err
	}
	return single(rows, "deleting session")
}

func (s sessionService) WaitUntilReady(ctx context.Context, scope Scope, id string) (string, error) {
	resp, err := api.PollGraphAnalyticsSessionReady(ctx, s.cfg, scope.OrgID, scope.ProjectID, id, api.GraphAnalyticsSessionWaitingStatus)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}
