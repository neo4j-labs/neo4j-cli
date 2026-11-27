// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clievents"
)

// Deployment is a Fleet Manager deployment. Record is the full server record,
// which is what commands render.
type Deployment struct {
	ID     string
	Name   string
	Status string

	Record map[string]any
}

// DeploymentService operates on Fleet Manager deployments, which are
// project-scoped: the deployments themselves plus their token, databases and
// servers. Every v2beta1 fleet-manager endpoint is covered; none is paginated.
type DeploymentService interface {
	List(ctx context.Context, scope Scope) ([]Deployment, error)
	Get(ctx context.Context, scope Scope, id string) (*Deployment, error)
	// Create registers an unmonitored deployment. connectionURL is optional.
	// The returned Record carries only the new deployment's id.
	Create(ctx context.Context, scope Scope, name, connectionURL string) (*Deployment, error)
	// Delete removes the deployment from Fleet Manager only; the running
	// database is untouched.
	Delete(ctx context.Context, scope Scope, id string) error

	// CreateToken mints the deployment's registration token. The token is
	// registered for redaction as soon as it is received.
	CreateToken(ctx context.Context, scope Scope, deploymentID string) (map[string]any, error)
	// RotateToken replaces the deployment's token (same redaction as CreateToken).
	RotateToken(ctx context.Context, scope Scope, deploymentID string) (map[string]any, error)
	DeleteToken(ctx context.Context, scope Scope, deploymentID string) error

	Databases(ctx context.Context, scope Scope, deploymentID string) ([]map[string]any, error)
	Servers(ctx context.Context, scope Scope, deploymentID string) ([]map[string]any, error)
	ServerDatabases(ctx context.Context, scope Scope, deploymentID, serverID string) ([]map[string]any, error)
}

type deploymentService struct {
	cfg *clicfg.Config
}

const deploymentsResource = "fleet-manager/deployments"

func newDeployment(rec map[string]any) Deployment {
	return Deployment{ID: str(rec, "id"), Name: str(rec, "name"), Status: str(rec, "status"), Record: rec}
}

func (s deploymentService) path(scope Scope, id string, suffix ...string) (string, error) {
	return scopedResourcePath(scope, deploymentsResource, "deployment", id, suffix...)
}

func (s deploymentService) rows(ctx context.Context, method, path string, body map[string]any, doing string, ok ...int) ([]map[string]any, error) {
	return v2Rows(ctx, s.cfg, method, path, body, nil, doing, ok...)
}

func (s deploymentService) List(ctx context.Context, scope Scope) ([]Deployment, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, http.MethodGet, p, nil, "listing deployments", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]Deployment, len(rows))
	for i, r := range rows {
		out[i] = newDeployment(r)
	}
	return out, nil
}

func (s deploymentService) Get(ctx context.Context, scope Scope, id string) (*Deployment, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.rows(ctx, http.MethodGet, p, nil, "fetching deployment", http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "fetching deployment")
	if err != nil {
		return nil, err
	}
	d := newDeployment(rec)
	return &d, nil
}

func (s deploymentService) Create(ctx context.Context, scope Scope, name, connectionURL string) (*Deployment, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	body := map[string]any{"name": name}
	if connectionURL != "" {
		body["connection_url"] = connectionURL
	}
	rows, err := s.rows(ctx, http.MethodPost, p, body, "creating deployment", http.StatusOK, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	d := newDeployment(rows[0])
	return &d, nil
}

func (s deploymentService) Delete(ctx context.Context, scope Scope, id string) error {
	p, err := s.path(scope, id)
	if err != nil {
		return err
	}
	return s.noContent(ctx, http.MethodDelete, p, "deleting deployment")
}

// noContent sends a request whose success carries no body (204; 200 tolerated).
func (s deploymentService) noContent(ctx context.Context, method, path, doing string) error {
	_, status, err := transport.MakeRequest(ctx, s.cfg, path, &transport.RequestConfig{Method: method, Version: transport.AuraApiVersion2})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("unexpected status %d %s", status, doing)
	}
	return nil
}

func (s deploymentService) token(ctx context.Context, method string, scope Scope, deploymentID, doing string) (map[string]any, error) {
	p, err := s.path(scope, deploymentID, "token")
	if err != nil {
		return nil, err
	}
	// The API takes an empty JSON object body for both token operations.
	rows, err := s.rows(ctx, method, p, map[string]any{}, doing, http.StatusOK, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	if tok, ok := rows[0]["token"].(string); ok {
		clievents.RegisterSecretValue(tok)
	}
	return rows[0], nil
}

func (s deploymentService) CreateToken(ctx context.Context, scope Scope, deploymentID string) (map[string]any, error) {
	return s.token(ctx, http.MethodPost, scope, deploymentID, "creating deployment token")
}

func (s deploymentService) RotateToken(ctx context.Context, scope Scope, deploymentID string) (map[string]any, error) {
	return s.token(ctx, http.MethodPatch, scope, deploymentID, "rotating deployment token")
}

func (s deploymentService) DeleteToken(ctx context.Context, scope Scope, deploymentID string) error {
	p, err := s.path(scope, deploymentID, "token")
	if err != nil {
		return err
	}
	return s.noContent(ctx, http.MethodDelete, p, "deleting deployment token")
}

func (s deploymentService) list(ctx context.Context, scope Scope, doing, deploymentID string, suffix ...string) ([]map[string]any, error) {
	p, err := s.path(scope, deploymentID, suffix...)
	if err != nil {
		return nil, err
	}
	return s.rows(ctx, http.MethodGet, p, nil, doing, http.StatusOK)
}

func (s deploymentService) Databases(ctx context.Context, scope Scope, deploymentID string) ([]map[string]any, error) {
	return s.list(ctx, scope, "listing deployment databases", deploymentID, "databases")
}

func (s deploymentService) Servers(ctx context.Context, scope Scope, deploymentID string) ([]map[string]any, error) {
	return s.list(ctx, scope, "listing deployment servers", deploymentID, "servers")
}

func (s deploymentService) ServerDatabases(ctx context.Context, scope Scope, deploymentID, serverID string) ([]map[string]any, error) {
	if err := ValidateResourceID("server", serverID); err != nil {
		return nil, err
	}
	return s.list(ctx, scope, "listing deployment server databases", deploymentID, "servers", serverID, "databases")
}
