// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package utils_test

import (
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithNotFoundContext(t *testing.T) {
	t.Parallel()

	const (
		resourceType = "snapshot"
		resourceID   = "snap-123"
		suggestion   = "Run 'neo4j-cli aura instance snapshot list --instance-id <id>' to see snapshots for this instance."
	)

	t.Run("not-found CLIError: mutates ResourceType, ResourceID, Suggestion", func(t *testing.T) {
		t.Parallel()

		original := clierr.NewNotFoundError("resource not found").WithResource("instance", "inst-1")
		got := utils.WithNotFoundContext(original, resourceType, resourceID, suggestion)

		var ce *clierr.CLIError
		require.True(t, errors.As(got, &ce))
		assert.Equal(t, 3, ce.Code)
		assert.Equal(t, resourceType, ce.ResourceType)
		assert.Equal(t, resourceID, ce.ResourceID)
		assert.Equal(t, suggestion, ce.Suggestion)
		// Same pointer — mutation in place.
		assert.Same(t, original, ce)
	})

	t.Run("non-404 CLIError: passes through unchanged", func(t *testing.T) {
		t.Parallel()

		original := clierr.NewAuthError("unauthorized")
		got := utils.WithNotFoundContext(original, resourceType, resourceID, suggestion)

		var ce *clierr.CLIError
		require.True(t, errors.As(got, &ce))
		assert.Equal(t, 4, ce.Code)
		assert.Empty(t, ce.ResourceType)
		assert.Empty(t, ce.ResourceID)
		assert.Empty(t, ce.Suggestion)
	})

	t.Run("non-CLIError: passes through unchanged", func(t *testing.T) {
		t.Parallel()

		original := errors.New("plain error")
		got := utils.WithNotFoundContext(original, resourceType, resourceID, suggestion)

		assert.Same(t, original, got)
	})

	t.Run("nil error: returns nil", func(t *testing.T) {
		t.Parallel()

		got := utils.WithNotFoundContext(nil, resourceType, resourceID, suggestion)
		assert.NoError(t, got)
	})
}
