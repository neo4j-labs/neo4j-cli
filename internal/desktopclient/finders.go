// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// Shared finders for picking the mutated entry out of the full catalog state
// that every /projects and /tags mutation returns (POST/PATCH/DELETE all
// respond with the post-mutation state, not the single entry). One policy for
// both catalogs — the mutated entry MUST be identifiable exactly once:
//
//   - absent from the returned state is a FATAL error: Desktop acknowledged
//     the mutation but its catalog disagrees, and rendering nothing (or a
//     literal `null` with exit 0) would report success for a change that may
//     not exist;
//   - a name matching several entries is a FATAL error listing the candidate
//     ids: the catalog holds duplicates, so printing any one of them could
//     present a stale pre-existing entry as the one just mutated.
//
// No first-match-wins and no newest-CreatedAt tiebreaks. Create paths locate
// by name; update paths locate by the resolved id.

// FindProjectByName locates the single project named `name` in a
// post-mutation ProjectsState. A nil state, no match, or more than one match
// is a fatal error (see the policy above).
func FindProjectByName(state *ProjectsState, name string) (*Project, error) {
	var items []Project
	if state != nil {
		items = state.Projects
	}
	return findOne(items, "project", name,
		func(p *Project) bool { return p.Name == name },
		func(p *Project) string { return p.ID })
}

// FindProjectByID locates the project with `id` in a post-mutation
// ProjectsState. A nil state or no match is a fatal error.
func FindProjectByID(state *ProjectsState, id string) (*Project, error) {
	var items []Project
	if state != nil {
		items = state.Projects
	}
	return findOne(items, "project", id,
		func(p *Project) bool { return p.ID == id },
		func(p *Project) string { return p.ID })
}

// FindTagByName locates the single tag named `name` in a post-mutation
// TagsState. A nil state, no match, or more than one match is a fatal error
// (see the policy above).
func FindTagByName(state *TagsState, name string) (*Tag, error) {
	var items []Tag
	if state != nil {
		items = state.Tags
	}
	return findOne(items, "tag", name,
		func(t *Tag) bool { return t.Name == name },
		func(t *Tag) string { return t.ID })
}

// FindTagByID locates the tag with `id` in a post-mutation TagsState. A nil
// state or no match is a fatal error.
func FindTagByID(state *TagsState, id string) (*Tag, error) {
	var items []Tag
	if state != nil {
		items = state.Tags
	}
	return findOne(items, "tag", id,
		func(t *Tag) bool { return t.ID == id },
		func(t *Tag) string { return t.ID })
}

// findOne is the shared core behind the Find* helpers: exactly one match is
// required. `selector` is the name or id that was looked up (echoed in error
// text); `match` tests one entry; `idOf` extracts an entry's id for the
// ambiguity message.
func findOne[T any](items []T, resource, selector string, match func(*T) bool, idOf func(*T) string) (*T, error) {
	var matches []*T
	for i := range items {
		if match(&items[i]) {
			matches = append(matches, &items[i])
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, clierr.NewFatalError(
			"desktop: %s %q is missing from the catalog state Desktop returned", resource, selector)
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			ids = append(ids, idOf(m))
		}
		return nil, clierr.NewFatalError(
			"desktop: %s %q matches %d entries in the catalog state Desktop returned (ids: %s) — the catalog holds duplicates, so the mutated entry cannot be identified",
			resource, selector, len(matches), strings.Join(ids, ", "))
	}
}
