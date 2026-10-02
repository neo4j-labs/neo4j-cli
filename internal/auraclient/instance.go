// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
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

// InstanceCreate describes an instance to provision. Type is the canonical
// v2beta1 tier name (free, professional, business-critical,
// virtual-dedicated-cloud); the v2beta1 endpoint rejects the v1 names. An empty
// Name is replaced by the lowest unused InstanceNN name in the project.
type InstanceCreate struct {
	Name                 string
	Version              string
	Region               string
	Type                 string
	CloudProvider        string
	Memory               string
	CustomerManagedKeyID string
	VectorOptimized      bool
	// GraphAnalyticsPlugin requests the graph analytics plugin (professional
	// tier only).
	GraphAnalyticsPlugin bool
}

// InstancePatch is a partial update: only non-empty fields are sent.
type InstancePatch struct {
	Name   string
	Memory string
}

// OverwriteSource names the data to overwrite an instance with. An empty
// InstanceID means the instance itself (restore from one of its own
// snapshots); SnapshotID is optional (latest when empty).
type OverwriteSource struct {
	InstanceID string
	SnapshotID string
}

type instanceService struct {
	cfg *clicfg.Config
}

func (s instanceService) Get(ctx context.Context, scope Scope, id string) (*Instance, error) {
	if err := ValidateResourceID("instance", id); err != nil {
		return nil, err
	}
	body, err := s.get(ctx, transport.ScopedInstancePath(scope.OrgID, scope.ProjectID, id), "fetching instance")
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

func (s instanceService) List(ctx context.Context, scope Scope) ([]Instance, error) {
	body, err := s.get(ctx, transport.ScopedInstancesPath(scope.OrgID, scope.ProjectID), "listing instances")
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

func (s instanceService) Delete(ctx context.Context, scope Scope, id string) (*Instance, error) {
	if err := ValidateResourceID("instance", id); err != nil {
		return nil, err
	}
	// v2beta1 answers 202 Accepted with the instance record.
	return s.mutate(ctx, http.MethodDelete, transport.ScopedInstancePath(scope.OrgID, scope.ProjectID, id), transport.AuraApiVersion2, "deleting instance")
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
	return s.mutate(ctx, http.MethodPost, fmt.Sprintf("/instances/%s/%s", id, action), "", action+" instance")
}

func (s instanceService) Verify(ctx context.Context, scope Scope, id string) error {
	if err := ValidateResourceID("instance", id); err != nil {
		return err
	}
	body, status, err := transport.MakeRequest(ctx, s.cfg, fmt.Sprintf("/instances/%s", id), &transport.RequestConfig{
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

func (s instanceService) WaitWhile(ctx context.Context, scope Scope, id, status string) (string, error) {
	resp, err := transport.PollInstance(ctx, s.cfg, scope.OrgID, scope.ProjectID, id, status)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}

func (s instanceService) get(ctx context.Context, path, doing string) ([]byte, error) {
	body, status, err := transport.MakeRequest(ctx, s.cfg, path, &transport.RequestConfig{
		Method:  http.MethodGet,
		Version: transport.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d %s", status, doing)
	}
	return body, nil
}

// mutate sends a body-less state-changing request; see send.
func (s instanceService) mutate(ctx context.Context, method, path string, version transport.AuraApiVersion, doing string) (*Instance, error) {
	return s.send(ctx, method, path, nil, version, doing)
}

// send issues a state-changing request that the API answers with 202 Accepted
// (200 is tolerated) and the instance record.
func (s instanceService) send(ctx context.Context, method, path string, reqBody map[string]any, version transport.AuraApiVersion, doing string) (*Instance, error) {
	body, status, err := transport.MakeRequest(ctx, s.cfg, path, &transport.RequestConfig{Method: method, PostBody: reqBody, Version: version})
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

func (s instanceService) Create(ctx context.Context, scope Scope, spec InstanceCreate) (*Instance, error) {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return nil, err
	}
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return nil, err
	}
	if spec.Name == "" {
		existing, err := s.List(ctx, scope)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(existing))
		for _, e := range existing {
			if e.Name != "" {
				names = append(names, e.Name)
			}
		}
		spec.Name = DefaultName("Instance", names)
	}

	body, err := s.send(ctx, http.MethodPost, transport.ScopedInstancesPath(scope.OrgID, scope.ProjectID), spec.body(scope.ProjectID), transport.AuraApiVersion2, "creating instance")
	if err != nil {
		return nil, err
	}
	// The password is returned once. Register it before anything can print or
	// capture it so tee files, telemetry and MCP transcripts scrub the literal.
	if pw, _ := body.Record["password"].(string); pw != "" {
		clievents.RegisterSecretValue(pw)
	}
	return body, nil
}

func (s instanceService) Update(ctx context.Context, scope Scope, id string, patch InstancePatch) (*Instance, error) {
	if err := s.Verify(ctx, scope, id); err != nil {
		return nil, err
	}
	body := map[string]any{}
	if patch.Memory != "" {
		body["memory"] = patch.Memory
	}
	if patch.Name != "" {
		body["name"] = patch.Name
	}
	return s.send(ctx, http.MethodPatch, "/instances/"+id, body, "", "updating instance")
}

func (s instanceService) Overwrite(ctx context.Context, scope Scope, id string, src OverwriteSource) (*Instance, error) {
	if err := s.Verify(ctx, scope, id); err != nil {
		return nil, err
	}
	if src.InstanceID == "" {
		src.InstanceID = id
	} else if err := ValidateResourceID("source instance", src.InstanceID); err != nil {
		return nil, err
	}
	body := map[string]any{"source_instance_id": src.InstanceID}
	if src.SnapshotID != "" {
		if err := ValidateResourceID("source snapshot", src.SnapshotID); err != nil {
			return nil, err
		}
		body["source_snapshot_id"] = src.SnapshotID
	}
	return s.send(ctx, http.MethodPost, "/instances/"+id+"/overwrite", body, "", "overwriting instance")
}

// body assembles the POST body from a validated spec. Free instances ignore the
// caller's memory, region, cloud provider and version and use fixed defaults,
// matching the Aura free-tier contract.
//
// graph_analytics is the v2beta1 enum, not the v1 "graph_analytics_plugin" bool
// (the scoped endpoint silently ignores the latter). Only the true case
// ("plugin") is sent: "unavailable" is not a settable create-time value, as
// every new instance gets at least "serverless", so the false case omits the
// field and lets the API apply that default.
func (c InstanceCreate) body(projectID string) map[string]any {
	body := map[string]any{
		"version":        c.Version,
		"region":         c.Region,
		"name":           c.Name,
		"type":           c.Type,
		"cloud_provider": c.CloudProvider,
		"tenant_id":      projectID,
	}

	if c.Type == "free" {
		body["memory"] = "1GB"
		body["region"] = "europe-west1"
		body["cloud_provider"] = "gcp"
		body["version"] = "5"
	} else {
		body["memory"] = c.Memory
		body["region"] = c.Region
		body["vector_optimized"] = c.VectorOptimized
	}

	if c.Type == "professional" && c.GraphAnalyticsPlugin {
		body["graph_analytics"] = "plugin"
	}

	if c.CustomerManagedKeyID != "" {
		body["customer_managed_key_id"] = c.CustomerManagedKeyID
	}

	return body
}
