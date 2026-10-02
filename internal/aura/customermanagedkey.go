// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// CustomerManagedKey is an Aura customer managed key. Record is the full server
// record (tenant_id exposed as project_id), which is what commands render.
type CustomerManagedKey struct {
	ID        string
	Name      string
	Status    string
	ProjectID string

	Record map[string]any
}

// CustomerManagedKeyCreate describes a key to register. InstanceType uses the
// v1 tier vocabulary (free-db, professional-db, enterprise-db, ...), which
// customer managed keys have not yet migrated away from.
type CustomerManagedKeyCreate struct {
	Name          string
	Region        string
	InstanceType  string
	CloudProvider string
	KeyID         string
}

// CustomerManagedKeyService operates on customer managed keys. The endpoints
// are v1 and not project-scoped; the service scopes them to the project itself.
type CustomerManagedKeyService interface {
	// List returns the project's keys.
	List(ctx context.Context, scope Scope) ([]CustomerManagedKey, error)
	// Get returns a key, or a not-found error unless it belongs to the project.
	// Callers that must check ownership before a destructive step use Get as
	// that check.
	Get(ctx context.Context, scope Scope, id string) (*CustomerManagedKey, error)
	Create(ctx context.Context, scope Scope, spec CustomerManagedKeyCreate) (*CustomerManagedKey, error)
	// Delete removes a key. It does not check ownership: call Get first.
	Delete(ctx context.Context, id string) error
	// WaitWhilePending polls until the key leaves its pending state and returns
	// the status it settled on.
	WaitWhilePending(ctx context.Context, id string) (string, error)
}

type cmkService struct {
	cfg *clicfg.Config
}

func newCMK(raw map[string]any) CustomerManagedKey {
	rec := RenameKey(raw, "tenant_id", "project_id", false)
	return CustomerManagedKey{ID: str(rec, "id"), Name: str(rec, "name"), Status: str(rec, "status"), ProjectID: str(rec, "project_id"), Record: rec}
}

func (s cmkService) rows(ctx context.Context, method, path string, body map[string]any, query map[string]string, doing string, ok ...int) ([]map[string]any, error) {
	resBody, status, err := api.MakeRequest(ctx, s.cfg, path, &api.RequestConfig{
		Method:      method,
		PostBody:    body,
		QueryParams: query,
	})
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
	return decodeRows(resBody)
}

func (s cmkService) List(ctx context.Context, scope Scope) ([]CustomerManagedKey, error) {
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, http.MethodGet, "/customer-managed-keys", nil, map[string]string{"tenantId": scope.ProjectID}, "listing customer-managed-keys", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]CustomerManagedKey, len(rows))
	for i, r := range rows {
		out[i] = newCMK(r)
	}
	return out, nil
}

func (s cmkService) Get(ctx context.Context, scope Scope, id string) (*CustomerManagedKey, error) {
	if err := ValidateResourceID("customer-managed-key", id); err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, http.MethodGet, "/customer-managed-keys/"+id, nil, nil, "from preflight ownership check", http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "fetching customer-managed-key")
	if err != nil {
		return nil, err
	}
	if tenantID, _ := rec["tenant_id"].(string); tenantID != scope.ProjectID {
		return nil, clierr.NewNotFoundError("could not find customer-managed-key %s in project %s", id, scope.ProjectID).
			WithResource("customer-managed-key", id).
			WithSuggestion("Run 'neo4j-cli aura customer-managed-key list --project-id <id>' to see keys in this project.")
	}
	k := newCMK(rec)
	return &k, nil
}

func (s cmkService) Create(ctx context.Context, scope Scope, spec CustomerManagedKeyCreate) (*CustomerManagedKey, error) {
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return nil, err
	}
	body := map[string]any{
		"region":         spec.Region,
		"name":           spec.Name,
		"instance_type":  spec.InstanceType,
		"cloud_provider": spec.CloudProvider,
		"key_id":         spec.KeyID,
		"tenant_id":      scope.ProjectID,
	}
	rows, err := s.rows(ctx, http.MethodPost, "/customer-managed-keys", body, nil, "creating customer-managed-key", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "creating customer-managed-key")
	if err != nil {
		return nil, err
	}
	k := newCMK(rec)
	return &k, nil
}

func (s cmkService) Delete(ctx context.Context, id string) error {
	if err := ValidateResourceID("customer-managed-key", id); err != nil {
		return err
	}
	_, status, err := api.MakeRequest(ctx, s.cfg, "/customer-managed-keys/"+id, &api.RequestConfig{Method: http.MethodDelete})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("unexpected status %d deleting customer-managed-key", status)
	}
	return nil
}

func (s cmkService) WaitWhilePending(ctx context.Context, id string) (string, error) {
	resp, err := api.PollCMK(ctx, s.cfg, id)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}
