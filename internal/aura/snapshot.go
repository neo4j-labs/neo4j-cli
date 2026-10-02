// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// Snapshot is a backup of an instance. Record is the full server record, which
// is what commands render.
type Snapshot struct {
	ID         string
	InstanceID string
	Status     string

	Record map[string]any
}

// SnapshotService operates on instance snapshots.
//
// These are instance-scoped v1 endpoints that are not project-scoped, so none
// of the methods check that the instance belongs to the caller's project. Call
// Instances().Verify (or utils.ResolveAndVerifyInstance) first.
type SnapshotService interface {
	// List returns an instance's snapshots, optionally only those taken on
	// date (YYYY-MM-DD).
	List(ctx context.Context, instanceID, date string) ([]Snapshot, error)
	// Get returns a snapshot; an absent one is a not-found error naming the
	// snapshot and suggesting how to list them.
	Get(ctx context.Context, instanceID, id string) (*Snapshot, error)
	// Create starts a snapshot and returns the record with its snapshot id.
	Create(ctx context.Context, instanceID string) (*Snapshot, error)
	// WaitWhilePending polls until the snapshot is neither pending nor in
	// progress and returns the status it settled on.
	WaitWhilePending(ctx context.Context, instanceID, id string) (string, error)
}

type snapshotService struct {
	cfg *clicfg.Config
}

func newSnapshot(rec map[string]any) Snapshot {
	return Snapshot{ID: str(rec, "snapshot_id"), InstanceID: str(rec, "instance_id"), Status: str(rec, "status"), Record: rec}
}

func (s snapshotService) path(instanceID, id string) (string, error) {
	if err := ValidateResourceID("instance", instanceID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/instances/%s/snapshots", instanceID)
	if id != "" {
		if err := ValidateResourceID("snapshot", id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	return p, nil
}

func (s snapshotService) rows(ctx context.Context, method, path string, query map[string]string, doing string, ok ...int) ([]map[string]any, error) {
	body, status, err := api.MakeRequest(ctx, s.cfg, path, &api.RequestConfig{Method: method, QueryParams: query})
	if err != nil {
		return nil, err
	}
	accepted := false
	for _, st := range ok {
		if status == st {
			accepted = true
		}
	}
	if !accepted {
		return nil, fmt.Errorf("unexpected status %d %s", status, doing)
	}
	return decodeRows(body)
}

func (s snapshotService) List(ctx context.Context, instanceID, date string) ([]Snapshot, error) {
	p, err := s.path(instanceID, "")
	if err != nil {
		return nil, err
	}
	var query map[string]string
	if date != "" {
		query = map[string]string{"date": date}
	}
	rows, err := s.rows(ctx, http.MethodGet, p, query, "listing snapshots", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, len(rows))
	for i, r := range rows {
		out[i] = newSnapshot(r)
	}
	return out, nil
}

func (s snapshotService) Get(ctx context.Context, instanceID, id string) (*Snapshot, error) {
	p, err := s.path(instanceID, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, http.MethodGet, p, nil, "fetching snapshot", http.StatusOK)
	if err != nil {
		return nil, withNotFoundContext(err, "snapshot", id, "Run 'neo4j-cli aura instance snapshot list --instance-id <id>' to see snapshots for this instance.")
	}
	rec, err := single(rows, "fetching snapshot")
	if err != nil {
		return nil, err
	}
	snap := newSnapshot(rec)
	return &snap, nil
}

func (s snapshotService) Create(ctx context.Context, instanceID string) (*Snapshot, error) {
	p, err := s.path(instanceID, "")
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, http.MethodPost, p, nil, "creating snapshot", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "creating snapshot")
	if err != nil {
		return nil, err
	}
	snap := newSnapshot(rec)
	return &snap, nil
}

func (s snapshotService) WaitWhilePending(ctx context.Context, instanceID, id string) (string, error) {
	resp, err := api.PollSnapshot(ctx, s.cfg, instanceID, id)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}

// withNotFoundContext rewrites a not-found (*clierr.CLIError with Code == 3) in
// place to carry the resource type, id and a next-action suggestion, and returns
// the same error. Any other error passes through unchanged. It is needed where
// the transport cannot derive the right resource from a nested URL path.
func withNotFoundContext(err error, resourceType, resourceID, suggestion string) error {
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 3 {
		return err
	}
	ce.ResourceType = resourceType
	ce.ResourceID = resourceID
	ce.Suggestion = suggestion
	return err
}
