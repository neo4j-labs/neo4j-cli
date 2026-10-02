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
	"github.com/neo4j/cli/internal/clievents"
)

// VirtualGraph statuses callers may wait on.
const VirtualGraphStatusCreating = transport.VirtualGraphStatusCreating

// VirtualGraph is an Aura virtual graph. Record is the full server record,
// which is what commands render.
type VirtualGraph struct {
	ID     string
	Name   string
	Status string

	Record map[string]any
}

// VirtualGraphCreate describes a virtual graph to create.
type VirtualGraphCreate struct {
	Name          string
	DataSourceID  string
	ImportModelID string
	CloudProvider string
	Region        string
	Memory        string // optional
	// MaximumBytesBilled is sent only when non-nil (BigQuery data sources).
	MaximumBytesBilled *int64
}

// VirtualGraphPatch is a partial update: only non-empty fields are sent.
type VirtualGraphPatch struct {
	Name          string
	Memory        string
	ImportModelID string
}

// VirtualGraphPage is one traversal of the list endpoint.
type VirtualGraphPage struct {
	Items []VirtualGraph
	// LimitReached reports that traversal stopped at the caller's limit while
	// more items were available.
	LimitReached bool
	// PageCapReached reports that traversal stopped at the page cap, so the
	// list may be incomplete.
	PageCapReached bool
}

// VirtualGraphService operates on virtual graphs, which are project-scoped.
type VirtualGraphService interface {
	// List walks every page, stopping after limit items when limit > 0.
	List(ctx context.Context, scope Scope, limit int) (*VirtualGraphPage, error)
	Get(ctx context.Context, scope Scope, id string) (*VirtualGraph, error)
	// Create returns the new virtual graph, including the one-time
	// plain_password in Record; the password is registered for redaction.
	Create(ctx context.Context, scope Scope, spec VirtualGraphCreate) (*VirtualGraph, error)
	// Update applies the patch and returns the virtual graph re-read afterwards.
	Update(ctx context.Context, scope Scope, id string, patch VirtualGraphPatch) (*VirtualGraph, error)
	Delete(ctx context.Context, scope Scope, id string) error
	// AllowedConfigs returns the memory configurations selectable for a new
	// virtual graph.
	AllowedConfigs(ctx context.Context, scope Scope) (map[string]any, error)
	// WaitWhile polls until the status is no longer status (case-insensitive)
	// and returns the status it settled on.
	WaitWhile(ctx context.Context, scope Scope, id, status string) (string, error)
}

type virtualGraphService struct {
	cfg *clicfg.Config
}

func newVirtualGraph(rec map[string]any) VirtualGraph {
	return VirtualGraph{ID: str(rec, "id"), Name: str(rec, "name"), Status: str(rec, "status"), Record: rec}
}

func (s virtualGraphService) scopeCheck(scope Scope) error {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return err
	}
	return ValidateResourceID("project", scope.ProjectID)
}

func (s virtualGraphService) request(ctx context.Context, method, path string, body map[string]any, doing string, ok ...int) ([]map[string]any, error) {
	return v2Rows(ctx, s.cfg, method, path, body, nil, doing, ok...)
}

func (s virtualGraphService) List(ctx context.Context, scope Scope, limit int) (*VirtualGraphPage, error) {
	if err := s.scopeCheck(scope); err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, clierr.NewUsageError("--limit must be zero or greater; zero returns every virtual graph")
	}
	res, err := transport.ListAllPages(ctx, s.cfg, transport.ScopedVirtualGraphsPath(scope.OrgID, scope.ProjectID), transport.AuraApiVersion2, limit)
	if err != nil {
		return nil, err
	}
	page := &VirtualGraphPage{LimitReached: res.LimitReached, PageCapReached: res.PageCapReached}
	for _, it := range res.Items {
		page.Items = append(page.Items, newVirtualGraph(it))
	}
	return page, nil
}

func (s virtualGraphService) Get(ctx context.Context, scope Scope, id string) (*VirtualGraph, error) {
	if err := s.scopeCheck(scope); err != nil {
		return nil, err
	}
	if err := ValidateResourceID("virtual-graph", id); err != nil {
		return nil, err
	}
	rows, err := s.request(ctx, http.MethodGet, transport.ScopedVirtualGraphPath(scope.OrgID, scope.ProjectID, id), nil, "fetching virtual graph", http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "fetching virtual graph")
	if err != nil {
		return nil, err
	}
	vg := newVirtualGraph(rec)
	return &vg, nil
}

func (s virtualGraphService) Create(ctx context.Context, scope Scope, spec VirtualGraphCreate) (*VirtualGraph, error) {
	if err := s.scopeCheck(scope); err != nil {
		return nil, err
	}
	body := map[string]any{
		"name":            spec.Name,
		"data_source_id":  spec.DataSourceID,
		"import_model_id": spec.ImportModelID,
		"cloud_provider":  spec.CloudProvider,
		"region":          spec.Region,
	}
	if spec.Memory != "" {
		body["memory"] = spec.Memory
	}
	if spec.MaximumBytesBilled != nil {
		body["maximum_bytes_billed"] = *spec.MaximumBytesBilled
	}
	rows, err := s.request(ctx, http.MethodPost, transport.ScopedVirtualGraphsPath(scope.OrgID, scope.ProjectID), body, "creating virtual graph", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	if password, ok := rows[0]["plain_password"].(string); ok {
		clievents.RegisterSecretValue(password)
	}
	vg := newVirtualGraph(rows[0])
	return &vg, nil
}

func (s virtualGraphService) Update(ctx context.Context, scope Scope, id string, patch VirtualGraphPatch) (*VirtualGraph, error) {
	if err := s.scopeCheck(scope); err != nil {
		return nil, err
	}
	if err := ValidateResourceID("virtual-graph", id); err != nil {
		return nil, err
	}
	body := map[string]any{}
	if patch.Name != "" {
		body["name"] = patch.Name
	}
	if patch.Memory != "" {
		body["memory"] = patch.Memory
	}
	if patch.ImportModelID != "" {
		body["import_model_id"] = patch.ImportModelID
	}
	// The PATCH response body is not used (it is not a full record, and may be
	// empty), so the result is re-read.
	_, status, err := transport.MakeRequest(ctx, s.cfg, transport.ScopedVirtualGraphPath(scope.OrgID, scope.ProjectID, id), &transport.RequestConfig{
		Method:   http.MethodPatch,
		PostBody: body,
		Version:  transport.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d updating virtual graph", status)
	}
	return s.Get(ctx, scope, id)
}

func (s virtualGraphService) Delete(ctx context.Context, scope Scope, id string) error {
	if err := s.scopeCheck(scope); err != nil {
		return err
	}
	if err := ValidateResourceID("virtual-graph", id); err != nil {
		return err
	}
	_, status, err := transport.MakeRequest(ctx, s.cfg, transport.ScopedVirtualGraphPath(scope.OrgID, scope.ProjectID, id), &transport.RequestConfig{
		Method:  http.MethodDelete,
		Version: transport.AuraApiVersion2,
	})
	if err != nil {
		return err
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return fmt.Errorf("unexpected status %d deleting virtual graph", status)
	}
	return nil
}

func (s virtualGraphService) AllowedConfigs(ctx context.Context, scope Scope) (map[string]any, error) {
	if err := s.scopeCheck(scope); err != nil {
		return nil, err
	}
	rows, err := s.request(ctx, http.MethodGet, transport.ScopedVirtualGraphAllowedConfigsPath(scope.OrgID, scope.ProjectID), nil, "fetching allowed configs", http.StatusOK)
	if err != nil {
		return nil, err
	}
	return single(rows, "fetching allowed configs")
}

func (s virtualGraphService) WaitWhile(ctx context.Context, scope Scope, id, status string) (string, error) {
	resp, err := transport.PollVirtualGraph(ctx, s.cfg, scope.OrgID, scope.ProjectID, id, status)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}

// v2Rows sends a request to a v2beta1 endpoint and decodes the {"data": ...}
// envelope into records. status must be one of ok. A non-GET request must come
// back with at least one record.
func v2Rows(ctx context.Context, cfg *clicfg.Config, method, path string, body map[string]any, query map[string]string, doing string, ok ...int) ([]map[string]any, error) {
	resBody, status, err := transport.MakeRequest(ctx, cfg, path, &transport.RequestConfig{
		Method:      method,
		PostBody:    body,
		QueryParams: query,
		Version:     transport.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}
	accepted := false
	for _, s := range ok {
		if status == s {
			accepted = true
		}
	}
	if !accepted {
		return nil, fmt.Errorf("unexpected status %d %s", status, doing)
	}
	rows, err := decodeRows(resBody)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && method != http.MethodGet {
		return nil, clierr.NewFatalError("unexpected response from Aura API: no record returned %s", doing)
	}
	return rows, nil
}

// single returns the only record of a single-resource response. An empty or
// multi-record "data" is an error rather than an index panic.
func single(rows []map[string]any, doing string) (map[string]any, error) {
	if len(rows) != 1 {
		return nil, clierr.NewFatalError("unexpected response from Aura API: expected 1 record %s, got %d", doing, len(rows))
	}
	return rows[0], nil
}
