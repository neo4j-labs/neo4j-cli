// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_ReturnsCuratedEntries(t *testing.T) {
	got := List()
	require.NotEmpty(t, got)

	bySlug := map[string]Suggestion{}
	for _, s := range got {
		bySlug[s.Slug] = s
	}

	// Slugs verified against the demo.neo4jlabs.com README.
	for _, slug := range []string{"movies", "recommendations", "northwind"} {
		assert.Contains(t, bySlug, slug, "expected curated slug %q", slug)
	}
}

func TestList_EntriesAreWellFormed(t *testing.T) {
	for _, tc := range List() {
		t.Run(tc.Slug, func(t *testing.T) {
			assert.NotEmpty(t, tc.Slug, "slug")
			assert.NotEmpty(t, tc.Title, "title")
			assert.NotEmpty(t, tc.Description, "description")
			require.NotEmpty(t, tc.OwnerRepo, "ownerRepo")

			owner, repo, ok := strings.Cut(tc.OwnerRepo, "/")
			require.True(t, ok, "ownerRepo %q must be owner/repo", tc.OwnerRepo)
			assert.NotEmpty(t, owner, "owner")
			assert.NotEmpty(t, repo, "repo")
		})
	}
}

func TestList_ReturnsACopy(t *testing.T) {
	first := List()
	require.NotEmpty(t, first)

	original := first[0].Title
	first[0].Title = "mutated"

	second := List()
	assert.Equal(t, original, second[0].Title, "List must not expose mutable internal state")
}
