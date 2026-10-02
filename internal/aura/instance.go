// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/aura/api"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// Instance is an Aura instance as the CLI sees it.
//
// The typed fields are for command logic. Record is the full server record
// after v2beta1 normalisation (legacy_status->status, tenant_id->project_id) and
// is what commands render, so fields this CLI does not model still reach the
// user's JSON output.
type Instance struct {
	ID             string
	Name           string
	Status         string
	OrganizationID string
	ProjectID      string
	ConnectionURL  string
	CloudProvider  string
	Region         string
	Type           string
	Memory         string
	Storage        string

	Record map[string]any
}

// normalizeV2Beta1 applies the renames shared by every v2beta1 resource:
// legacy_status->status and tenant_id->project_id, a native value winning over
// its legacy twin. The input is not modified.
func normalizeV2Beta1(raw map[string]any) map[string]any {
	rec := RenameKey(raw, "legacy_status", "status", true)
	return RenameKey(rec, "tenant_id", "project_id", true)
}

func newInstance(raw map[string]any) Instance {
	rec := normalizeV2Beta1(raw)
	return Instance{
		ID:             str(rec, "id"),
		Name:           str(rec, "name"),
		Status:         str(rec, "status"),
		OrganizationID: str(rec, "organization_id"),
		ProjectID:      str(rec, "project_id"),
		ConnectionURL:  str(rec, "connection_url"),
		CloudProvider:  str(rec, "cloud_provider"),
		Region:         str(rec, "region"),
		Type:           str(rec, "type"),
		Memory:         str(rec, "memory"),
		Storage:        str(rec, "storage"),
		Record:         rec,
	}
}

type instanceService struct {
	cfg *clicfg.Config
}

func (s instanceService) Get(_ context.Context, scope Scope, id string) (*Instance, error) {
	if err := ValidateResourceID("instance", id); err != nil {
		return nil, err
	}
	body, err := s.get(api.ScopedInstancePath(scope.OrgID, scope.ProjectID, id), "fetching instance")
	if err != nil {
		return nil, err
	}
	rows, err := decodeRows(body)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, clierr.NewFatalError("expected 1 array value: %v", len(rows))
	}
	inst := newInstance(rows[0])
	return &inst, nil
}

func (s instanceService) List(_ context.Context, scope Scope) ([]Instance, error) {
	body, err := s.get(api.ScopedInstancesPath(scope.OrgID, scope.ProjectID), "listing instances")
	if err != nil {
		return nil, err
	}
	rows, err := decodeRows(body)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, len(rows))
	for i, row := range rows {
		inst := newInstance(row)
		if _, ok := inst.Record["organization_id"]; !ok {
			inst.Record["organization_id"] = scope.OrgID
			inst.OrganizationID = scope.OrgID
		}
		out[i] = inst
	}
	return out, nil
}

func (s instanceService) Delete(_ context.Context, scope Scope, id string) (*Instance, error) {
	if err := ValidateResourceID("instance", id); err != nil {
		return nil, err
	}
	// v2beta1 answers 202 Accepted with the instance record.
	return s.mutate(http.MethodDelete, api.ScopedInstancePath(scope.OrgID, scope.ProjectID, id), api.AuraApiVersion2, "deleting instance")
}

// Pause and Resume are still v1 endpoints that are not project-scoped, so
// ownership is verified first.
func (s instanceService) Pause(ctx context.Context, scope Scope, id string) (*Instance, error) {
	return s.transition(ctx, scope, id, "pause")
}

func (s instanceService) Resume(ctx context.Context, scope Scope, id string) (*Instance, error) {
	return s.transition(ctx, scope, id, "resume")
}

func (s instanceService) transition(ctx context.Context, scope Scope, id, action string) (*Instance, error) {
	if err := s.Verify(ctx, scope, id); err != nil {
		return nil, err
	}
	return s.mutate(http.MethodPost, fmt.Sprintf("/instances/%s/%s", id, action), "", action+" instance")
}

func (s instanceService) Verify(_ context.Context, scope Scope, id string) error {
	if err := ValidateResourceID("instance", id); err != nil {
		return err
	}
	body, status, err := api.MakeRequest(s.cfg, fmt.Sprintf("/instances/%s", id), &api.RequestConfig{
		Method: http.MethodGet,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected status %d from preflight ownership check", status)
	}
	rows, err := decodeRows(body)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return clierr.NewFatalError("expected 1 array value: %v", len(rows))
	}
	if projectID, _ := rows[0]["tenant_id"].(string); projectID != scope.ProjectID {
		return clierr.NewNotFoundError("could not find instance %s in project %s", id, scope.ProjectID).
			WithResource("instance", id).
			WithSuggestion("Run 'neo4j-cli aura instance list --project-id <id>' to see instances in this project.")
	}
	return nil
}

func (s instanceService) WaitWhile(_ context.Context, scope Scope, id, status string) (string, error) {
	resp, err := api.PollInstance(s.cfg, scope.OrgID, scope.ProjectID, id, status)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}

func (s instanceService) get(path, doing string) ([]byte, error) {
	body, status, err := api.MakeRequest(s.cfg, path, &api.RequestConfig{
		Method:  http.MethodGet,
		Version: api.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d %s", status, doing)
	}
	return body, nil
}

// mutate sends a state-changing request that the API answers with 202 Accepted
// (200 is tolerated) and the instance record.
func (s instanceService) mutate(method, path string, version api.AuraApiVersion, doing string) (*Instance, error) {
	body, status, err := api.MakeRequest(s.cfg, path, &api.RequestConfig{Method: method, Version: version})
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d %s", status, doing)
	}
	rows, err := decodeRows(body)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, clierr.NewFatalError("expected 1 array value: %v", len(rows))
	}
	inst := newInstance(rows[0])
	return &inst, nil
}

// decodeRows reads the {"data": ...} envelope, where data is either an array of
// records or a single record (returned as one row). Malformed JSON is an error,
// not a panic.
func decodeRows(body []byte) ([]map[string]any, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, clierr.NewFatalError("unexpected response from Aura API: %s", err.Error())
	}
	var rows []map[string]any
	if err := json.Unmarshal(env.Data, &rows); err == nil {
		return rows, nil
	}
	var row map[string]any
	if err := json.Unmarshal(env.Data, &row); err != nil {
		return nil, clierr.NewFatalError("unexpected response from Aura API: %s", err.Error())
	}
	return []map[string]any{row}, nil
}
