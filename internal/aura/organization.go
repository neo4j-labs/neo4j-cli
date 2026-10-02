// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"context"
	"net/http"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// Organization is an Aura organization. Record is the full server record,
// which is what commands render.
type Organization struct {
	ID   string
	Name string

	Record map[string]any
}

// Project is an Aura project (the v2beta1 successor of a tenant).
type Project struct {
	ID   string
	Name string

	Record map[string]any
}

// OrganizationService reads the organizations the caller can access.
type OrganizationService interface {
	List(ctx context.Context) ([]Organization, error)
	Get(ctx context.Context, id string) (*Organization, error)
}

// ProjectService reads the projects of an organization.
type ProjectService interface {
	List(ctx context.Context, orgID string) ([]Project, error)
	// Get returns one project, or a not-found error carrying the resource and a
	// suggestion. It is derived from List: that is the one endpoint the CLI
	// relies on for project lookup and scope validation.
	Get(ctx context.Context, orgID, projectID string) (*Project, error)
}

type organizationService struct {
	cfg *clicfg.Config
}

type projectService struct {
	cfg *clicfg.Config
}

func newOrganization(rec map[string]any) Organization {
	return Organization{ID: str(rec, "id"), Name: str(rec, "name"), Record: rec}
}

func newProject(rec map[string]any) Project {
	return Project{ID: str(rec, "id"), Name: str(rec, "name"), Record: rec}
}

func (s organizationService) List(ctx context.Context) ([]Organization, error) {
	rows, err := v2Rows(ctx, s.cfg, http.MethodGet, "/organizations", nil, nil, "listing organizations", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]Organization, len(rows))
	for i, r := range rows {
		out[i] = newOrganization(r)
	}
	return out, nil
}

func (s organizationService) Get(ctx context.Context, id string) (*Organization, error) {
	if err := ValidateResourceID("organization", id); err != nil {
		return nil, err
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodGet, "/organizations/"+id, nil, nil, "fetching organization", http.StatusOK)
	if err != nil {
		return nil, err
	}
	rec, err := single(rows, "fetching organization")
	if err != nil {
		return nil, err
	}
	org := newOrganization(rec)
	return &org, nil
}

func (s projectService) List(ctx context.Context, orgID string) ([]Project, error) {
	if err := ValidateResourceID("organization", orgID); err != nil {
		return nil, err
	}
	rows, err := v2Rows(ctx, s.cfg, http.MethodGet, "/organizations/"+orgID+"/projects", nil, nil, "listing projects", http.StatusOK)
	if err != nil {
		return nil, err
	}
	out := make([]Project, len(rows))
	for i, r := range rows {
		out[i] = newProject(r)
	}
	return out, nil
}

func (s projectService) Get(ctx context.Context, orgID, projectID string) (*Project, error) {
	projects, err := s.List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == projectID {
			return &projects[i], nil
		}
	}
	return nil, clierr.NewNotFoundError("could not find project %s in organization %s", projectID, orgID).
		WithResource("project", projectID).
		WithSuggestion("Run 'neo4j-cli aura project list --organization-id <id>' to see available projects.")
}
