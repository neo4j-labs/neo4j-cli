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

func newInstance(raw map[string]any) Instance {
	rec := RenameKey(raw, "legacy_status", "status", true)
	rec = RenameKey(rec, "tenant_id", "project_id", true)
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
