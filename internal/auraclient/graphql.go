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

// GraphQL Data API statuses callers may wait on.
const (
	GraphQLStatusCreating = transport.GraphQLDataApiStatusCreating
	GraphQLStatusUpdating = transport.GraphQLDataApiStatusUpdating
	GraphQLStatusPausing  = transport.GraphQLDataApiStatusPausing
	GraphQLStatusResuming = transport.GraphQLDataApiStatusResuming
)

// Authentication provider types.
const (
	AuthProviderAPIKey = transport.GraphQLDataApiAuthProviderTypeApiKey
	AuthProviderJWKS   = transport.GraphQLDataApiAuthProviderTypeJwks
)

// GraphQLDataAPI is a GraphQL Data API attached to an Aura instance. Record is
// the full server record, which is what commands render.
type GraphQLDataAPI struct {
	ID     string
	Name   string
	Status string
	URL    string

	Record map[string]any
}

// GraphQLCreate describes a Data API to create. An empty Name is replaced by
// the lowest unused GraphQLNN name on the instance.
type GraphQLCreate struct {
	Name            string
	Memory          string
	ServiceAccount  string
	TypeDefinitions string // base64-encoded
}

// GraphQLPatch is a partial update: only non-empty fields are sent.
type GraphQLPatch struct {
	Name            string
	TypeDefinitions string // base64-encoded
	ServiceAccount  string
}

// AuthProvider is an authentication provider of a GraphQL Data API.
type AuthProvider struct {
	ID      string
	Name    string
	Type    string
	Enabled bool
	URL     string
	// Key is the API key of a freshly created api-key provider. It is only
	// returned on creation and is registered for redaction as soon as it is
	// received.
	Key string

	Record map[string]any
}

// AuthProviderSpec describes an authentication provider to create.
type AuthProviderSpec struct {
	Type    string
	Name    string
	Enabled bool
	URL     string // JWKS validation URL; empty for api-key
}

// GraphQLService operates on GraphQL Data APIs.
//
// These are instance-scoped endpoints that are not project-scoped, so none of
// the methods check that the instance belongs to the caller's project. Call
// Instances().Verify (or utils.ResolveAndVerifyInstance) first. Create, Update,
// Delete, Pause, Resume and SetAllowedOrigins return the record the API sent
// back; the Data API is usually still transitioning, so use WaitWhile to block
// until it has finished.
type GraphQLService interface {
	List(ctx context.Context, instanceID string) ([]GraphQLDataAPI, error)
	Get(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error)
	Create(ctx context.Context, instanceID string, spec GraphQLCreate) (*GraphQLDataAPI, error)
	Update(ctx context.Context, instanceID, id string, patch GraphQLPatch) (*GraphQLDataAPI, error)
	Delete(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error)
	Pause(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error)
	Resume(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error)

	// AllowedOrigins returns the CORS allowed origins of the Data API.
	AllowedOrigins(ctx context.Context, instanceID, id string) ([]string, error)
	// SetAllowedOrigins replaces the CORS allowed origins (empty clears them).
	SetAllowedOrigins(ctx context.Context, instanceID, id string, origins []string) (*GraphQLDataAPI, error)

	// WaitWhile polls until the Data API's status is no longer status and
	// returns the status it settled on.
	WaitWhile(ctx context.Context, instanceID, id, status string) (string, error)

	AuthProviders() AuthProviderService
}

// AuthProviderService operates on the authentication providers of one Data API.
// The same project-ownership caveat as GraphQLService applies.
type AuthProviderService interface {
	List(ctx context.Context, instanceID, dataAPIID string) ([]AuthProvider, error)
	Get(ctx context.Context, instanceID, dataAPIID, id string) (*AuthProvider, error)
	Create(ctx context.Context, instanceID, dataAPIID string, spec AuthProviderSpec) (*AuthProvider, error)
	Delete(ctx context.Context, instanceID, dataAPIID, id string) (*AuthProvider, error)
}

type graphqlService struct {
	cfg *clicfg.Config
}

func (s graphqlService) AuthProviders() AuthProviderService { return authProviderService(s) }

func newGraphQL(rec map[string]any) GraphQLDataAPI {
	return GraphQLDataAPI{ID: str(rec, "id"), Name: str(rec, "name"), Status: str(rec, "status"), URL: str(rec, "url"), Record: rec}
}

func (s graphqlService) path(instanceID, id string, suffix ...string) (string, error) {
	if err := ValidateResourceID("instance", instanceID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/instances/%s/data-apis/graphql", instanceID)
	if id != "" {
		if err := ValidateResourceID("graphql data api", id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	for _, sfx := range suffix {
		p += "/" + sfx
	}
	return p, nil
}

// List returns the instance's GraphQL data APIs. The beta API declares no
// pagination parameters for this endpoint
// (GET /instances/{id}/data-apis/graphql); one GET returns the full
// collection.
func (s graphqlService) List(ctx context.Context, instanceID string) ([]GraphQLDataAPI, error) {
	p, err := s.path(instanceID, "")
	if err != nil {
		return nil, err
	}
	rows, err := betaRows(ctx, s.cfg, http.MethodGet, p, nil, "listing graphql data apis", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]GraphQLDataAPI, len(rows))
	for i, r := range rows {
		out[i] = newGraphQL(r)
	}
	return out, nil
}

func (s graphqlService) Get(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodGet, p, nil, "fetching graphql data api", http.StatusOK)
}

func (s graphqlService) Create(ctx context.Context, instanceID string, spec GraphQLCreate) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, "")
	if err != nil {
		return nil, err
	}
	name := spec.Name
	if name == "" {
		existing, err := s.List(ctx, instanceID)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(existing))
		for _, e := range existing {
			if e.Name != "" {
				names = append(names, e.Name)
			}
		}
		name = DefaultName("GraphQL", names)
	}
	body := spec.body(name)
	rows, err := betaRows(ctx, s.cfg, http.MethodPost, p, body, "creating graphql data api", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	registerProviderKeys(rows)
	g := newGraphQL(rows[0])
	return &g, nil
}

func (s graphqlService) Update(ctx context.Context, instanceID, id string, patch GraphQLPatch) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, id)
	if err != nil {
		return nil, err
	}
	body := patch.body()
	return s.one(ctx, http.MethodPatch, p, body, "updating graphql data api", http.StatusAccepted, http.StatusOK)
}

func (s graphqlService) Delete(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodDelete, p, nil, "deleting graphql data api", http.StatusAccepted, http.StatusOK)
}

func (s graphqlService) Pause(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, id, "pause")
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodPost, p, nil, "pausing graphql data api", http.StatusAccepted, http.StatusOK)
}

func (s graphqlService) Resume(ctx context.Context, instanceID, id string) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, id, "resume")
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodPost, p, nil, "resuming graphql data api", http.StatusAccepted, http.StatusOK)
}

func (s graphqlService) AllowedOrigins(ctx context.Context, instanceID, id string) ([]string, error) {
	p, err := s.path(instanceID, id)
	if err != nil {
		return nil, err
	}
	body, status, err := transport.MakeRequest(ctx, s.cfg, p, &transport.RequestConfig{Method: http.MethodGet, Version: transport.AuraApiVersionBeta1})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, clierr.NewUpstreamError("unexpected status code %d from GraphQL Data API CORS policy response: %s", status, transport.ScrubbedBodyTrunc(body))
	}
	var parsed struct {
		Data struct {
			Security struct {
				CorsPolicy struct {
					AllowedOrigins []string `json:"allowed_origins"`
				} `json:"cors_policy"`
			} `json:"security"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, clierr.NewUpstreamError("could not parse GraphQL Data API CORS policy response: %s", transport.ScrubbedBodyTrunc(body))
	}
	return parsed.Data.Security.CorsPolicy.AllowedOrigins, nil
}

func (s graphqlService) SetAllowedOrigins(ctx context.Context, instanceID, id string, origins []string) (*GraphQLDataAPI, error) {
	p, err := s.path(instanceID, id)
	if err != nil {
		return nil, err
	}
	body := originsBody(origins)
	return s.one(ctx, http.MethodPatch, p, body, "updating graphql data api", http.StatusAccepted, http.StatusOK)
}

func (s graphqlService) WaitWhile(ctx context.Context, instanceID, id, status string) (string, error) {
	if err := ValidateResourceID("instance", instanceID); err != nil {
		return "", err
	}
	if err := ValidateResourceID("graphql data api", id); err != nil {
		return "", err
	}
	resp, err := transport.PollGraphQLDataApi(ctx, s.cfg, instanceID, id, status)
	if err != nil {
		return "", err
	}
	return resp.Data.Status, nil
}

func (s graphqlService) one(ctx context.Context, method, path string, body map[string]any, doing string, ok ...int) (*GraphQLDataAPI, error) {
	rows, err := betaRows(ctx, s.cfg, method, path, body, doing, ok...)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, doing)
	if err != nil {
		return nil, err
	}
	g := newGraphQL(rec)
	return &g, nil
}

type authProviderService struct {
	cfg *clicfg.Config
}

func newAuthProvider(rec map[string]any) AuthProvider {
	enabled, _ := rec["enabled"].(bool)
	return AuthProvider{ID: str(rec, "id"), Name: str(rec, "name"), Type: str(rec, "type"), Enabled: enabled, URL: str(rec, "url"), Key: str(rec, "key"), Record: rec}
}

func (s authProviderService) path(instanceID, dataAPIID, id string) (string, error) {
	if err := ValidateResourceID("instance", instanceID); err != nil {
		return "", err
	}
	if err := ValidateResourceID("graphql data api", dataAPIID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/instances/%s/data-apis/graphql/%s/auth-providers", instanceID, dataAPIID)
	if id != "" {
		if err := ValidateResourceID("auth provider", id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	return p, nil
}

// List returns the data API's auth providers. The beta API declares no
// pagination parameters for this endpoint
// (GET /instances/{id}/data-apis/graphql/{apiId}/auth-providers); one GET
// returns the full collection.
func (s authProviderService) List(ctx context.Context, instanceID, dataAPIID string) ([]AuthProvider, error) {
	p, err := s.path(instanceID, dataAPIID, "")
	if err != nil {
		return nil, err
	}
	rows, err := betaRows(ctx, s.cfg, http.MethodGet, p, nil, "listing auth providers", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]AuthProvider, len(rows))
	for i, r := range rows {
		out[i] = newAuthProvider(r)
	}
	return out, nil
}

func (s authProviderService) Get(ctx context.Context, instanceID, dataAPIID, id string) (*AuthProvider, error) {
	p, err := s.path(instanceID, dataAPIID, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodGet, p, nil, "fetching auth provider", http.StatusOK)
}

func (s authProviderService) Create(ctx context.Context, instanceID, dataAPIID string, spec AuthProviderSpec) (*AuthProvider, error) {
	p, err := s.path(instanceID, dataAPIID, "")
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"type":    spec.Type,
		"name":    spec.Name,
		"enabled": spec.Enabled,
	}
	if spec.URL != "" {
		body["url"] = spec.URL
	}
	rows, err := betaRows(ctx, s.cfg, http.MethodPost, p, body, "creating auth provider", http.StatusAccepted, http.StatusOK)
	if err != nil {
		return nil, err
	}
	registerProviderKeys(rows)
	ap := newAuthProvider(rows[0])
	return &ap, nil
}

func (s authProviderService) Delete(ctx context.Context, instanceID, dataAPIID, id string) (*AuthProvider, error) {
	p, err := s.path(instanceID, dataAPIID, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodDelete, p, nil, "deleting auth provider", http.StatusAccepted, http.StatusOK)
}

func (s authProviderService) one(ctx context.Context, method, path string, body map[string]any, doing string, ok ...int) (*AuthProvider, error) {
	rows, err := betaRows(ctx, s.cfg, method, path, body, doing, ok...)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, doing)
	if err != nil {
		return nil, err
	}
	ap := newAuthProvider(rec)
	return &ap, nil
}

// betaRows sends a request to a v1beta1 endpoint and decodes the {"data": ...}
// envelope. status must be one of ok; the result always has at least one row
// unless a list was requested with GET (which may legitimately be empty).
func betaRows(ctx context.Context, cfg *clicfg.Config, method, path string, body map[string]any, doing string, ok ...int) ([]map[string]any, error) {
	resBody, status, err := transport.MakeRequest(ctx, cfg, path, &transport.RequestConfig{
		Method:   method,
		PostBody: body,
		Version:  transport.AuraApiVersionBeta1,
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

// registerProviderKeys registers any API key found in the records so it is
// scrubbed from output captured for telemetry, tee files and MCP transcripts.
// Records are either a Data API (keys under authentication_providers) or a
// single authentication provider (key at the top level).
func registerProviderKeys(rows []map[string]any) {
	for _, r := range rows {
		if k, _ := r["key"].(string); k != "" {
			clievents.RegisterSecretValue(k)
		}
		providers, _ := r["authentication_providers"].([]any)
		for _, p := range providers {
			if m, ok := p.(map[string]any); ok {
				if k, _ := m["key"].(string); k != "" {
					clievents.RegisterSecretValue(k)
				}
			}
		}
	}
}

func (c GraphQLCreate) body(name string) map[string]any {
	return map[string]any{
		"name":   name,
		"memory": c.Memory,
		"aura_instance": map[string]string{
			"service_account": c.ServiceAccount,
		},
		"security": map[string]any{
			"authentication_providers": []map[string]any{
				{"type": "api-key", "name": "default", "enabled": true},
			},
		},
		"type_definitions": c.TypeDefinitions,
	}
}

func (p GraphQLPatch) body() map[string]any {
	body := map[string]any{}
	if p.Name != "" {
		body["name"] = p.Name
	}
	if p.TypeDefinitions != "" {
		body["type_definitions"] = p.TypeDefinitions
	}
	if p.ServiceAccount != "" {
		body["aura_instance"] = map[string]string{"service_account": p.ServiceAccount}
	}
	return body
}

func originsBody(origins []string) map[string]any {
	body := map[string]any{
		"security": map[string]any{
			"cors_policy": map[string]any{
				"allowed_origins": origins,
			},
		},
	}
	if len(origins) == 0 {
		// The API rejects a PATCH whose only change is an empty list, so an
		// ignored extra field is sent along with it.
		body["test"] = "ignore me"
	}
	return body
}
