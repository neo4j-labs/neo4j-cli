// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/neo4j/cli/internal/clierr"
)

// This file covers Desktop's project + tag catalogs (`/projects`, `/tags`)
// and the name→ID resolvers built on top of them. Unlike the `/dbmss` and
// `/connections` routes, these routes identify the mutated entry in the BODY
// (PATCH) or QUERYSTRING (DELETE) — never as a path segment — and every
// mutation responds with the full post-mutation state, not the single entry.

// ListProjects returns the project catalog (`GET /projects`). The
// `currentProject` selection is Desktop-UI state and intentionally not
// surfaced here.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	body, err := c.do(ctx, http.MethodGet, "/projects", nil)
	if err != nil {
		return nil, err
	}
	out, err := decodeState[ProjectsState](body, "project list")
	if err != nil {
		return nil, err
	}
	return out.Projects, nil
}

// CreateProject posts `{"name": ...}` and returns the full post-create state.
func (c *Client) CreateProject(ctx context.Context, name string) (*ProjectsState, error) {
	body, err := c.do(ctx, http.MethodPost, "/projects", map[string]any{"name": name})
	if err != nil {
		return nil, err
	}
	return decodeState[ProjectsState](body, "created project state")
}

// UpdateProject renames a project. The id rides in the BODY (`{"id","name"}`)
// — the route has no path parameter.
func (c *Client) UpdateProject(ctx context.Context, id, name string) (*ProjectsState, error) {
	body, err := c.do(ctx, http.MethodPatch, "/projects", map[string]any{"id": id, "name": name})
	if err != nil {
		return nil, err
	}
	return decodeState[ProjectsState](body, "updated project state")
}

// DeleteProject removes one project. The id rides in the QUERYSTRING
// (`DELETE /projects?id=<uuid>`) with no body.
func (c *Client) DeleteProject(ctx context.Context, id string) (*ProjectsState, error) {
	body, err := c.do(ctx, http.MethodDelete, "/projects?id="+url.QueryEscape(id), nil)
	if err != nil {
		return nil, err
	}
	return decodeState[ProjectsState](body, "deleted project state")
}

// decodeState is the shared JSON decoder for the full-catalog-state bodies
// that every /projects and /tags route responds with. `what` names the
// decoded payload for the error message (e.g. "created project state").
func decodeState[T any](body []byte, what string) (*T, error) {
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode %s: %s", what, err.Error())
	}
	return &out, nil
}

// ListTags returns the tag catalog (`GET /tags`). The UI `filter` selection
// is intentionally not surfaced here.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	body, err := c.do(ctx, http.MethodGet, "/tags", nil)
	if err != nil {
		return nil, err
	}
	out, err := decodeState[TagsState](body, "tag list")
	if err != nil {
		return nil, err
	}
	return out.Tags, nil
}

// CreateTag posts `{"name": ..., "color": ...}`; an empty color omits the key
// (Desktop then picks a default). Returns the full post-create state.
func (c *Client) CreateTag(ctx context.Context, name, color string) (*TagsState, error) {
	payload := map[string]any{"name": name}
	if color != "" {
		payload["color"] = color
	}
	body, err := c.do(ctx, http.MethodPost, "/tags", payload)
	if err != nil {
		return nil, err
	}
	return decodeState[TagsState](body, "created tag state")
}

// UpdateTag edits a tag. The id rides in the BODY (`{"id","name","color"?}`)
// — the route has no path parameter. An empty color omits the key.
func (c *Client) UpdateTag(ctx context.Context, id, name, color string) (*TagsState, error) {
	payload := map[string]any{"id": id, "name": name}
	if color != "" {
		payload["color"] = color
	}
	body, err := c.do(ctx, http.MethodPatch, "/tags", payload)
	if err != nil {
		return nil, err
	}
	return decodeState[TagsState](body, "updated tag state")
}

// DeleteTag removes one tag. The id rides in the QUERYSTRING
// (`DELETE /tags?id=<uuid>`) with no body.
func (c *Client) DeleteTag(ctx context.Context, id string) (*TagsState, error) {
	body, err := c.do(ctx, http.MethodDelete, "/tags?id="+url.QueryEscape(id), nil)
	if err != nil {
		return nil, err
	}
	return decodeState[TagsState](body, "deleted tag state")
}

// NonEmptyStrings drops empty-string elements (StringSlice quirks like
// `--tags ""` yield one empty element) so they never reach name resolution;
// an all-empty input collapses to an empty slice, which clears the set
// server-side.
func NonEmptyStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ResolveProjectIDs maps each entry of `namesOrIDs` to a project ID: values
// that parse as UUIDs pass through verbatim; anything else is matched
// case-sensitively against the ListProjects catalog by name. Unknown names
// and names matching more than one project are usage errors.
func (c *Client) ResolveProjectIDs(ctx context.Context, namesOrIDs []string) ([]string, error) {
	return resolveIDs(namesOrIDs, "project", func() ([]namedID, error) {
		projects, err := c.ListProjects(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]namedID, 0, len(projects))
		for _, p := range projects {
			out = append(out, namedID{id: p.ID, name: p.Name})
		}
		return out, nil
	})
}

// ResolveTagIDs maps each entry of `namesOrIDs` to a tag ID: values that
// parse as UUIDs pass through verbatim; anything else is matched
// case-sensitively against the ListTags catalog by name. Unknown names and
// names matching more than one tag are usage errors.
func (c *Client) ResolveTagIDs(ctx context.Context, namesOrIDs []string) ([]string, error) {
	return resolveIDs(namesOrIDs, "tag", func() ([]namedID, error) {
		tags, err := c.ListTags(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]namedID, 0, len(tags))
		for _, t := range tags {
			out = append(out, namedID{id: t.ID, name: t.Name})
		}
		return out, nil
	})
}

type namedID struct {
	id   string
	name string
}

// resolveIDs is the shared name→ID resolution behind ResolveProjectIDs /
// ResolveTagIDs. The catalog is fetched lazily — an all-UUID input never
// hits the network.
func resolveIDs(namesOrIDs []string, resource string, catalog func() ([]namedID, error)) ([]string, error) {
	out := make([]string, 0, len(namesOrIDs))
	var entries []namedID
	fetched := false
	for _, in := range namesOrIDs {
		if _, err := uuid.Parse(in); err == nil {
			out = append(out, in)
			continue
		}
		if !fetched {
			var err error
			entries, err = catalog()
			if err != nil {
				return nil, err
			}
			fetched = true
		}
		var matches []namedID
		for _, e := range entries {
			if e.name == in {
				matches = append(matches, e)
			}
		}
		switch len(matches) {
		case 0:
			return nil, clierr.NewUsageError("unknown %s %q — pass a %s ID or an exact %s name from the Desktop catalog", resource, in, resource, resource)
		case 1:
			out = append(out, matches[0].id)
		default:
			ids := make([]string, 0, len(matches))
			for _, m := range matches {
				ids = append(ids, m.id)
			}
			return nil, clierr.NewUsageError("ambiguous %s name %q matches %d %ss (%s) — pass one of the IDs instead", resource, in, len(matches), resource, strings.Join(ids, ", "))
		}
	}
	return out, nil
}
