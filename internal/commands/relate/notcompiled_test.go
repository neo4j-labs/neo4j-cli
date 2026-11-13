// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !relate

// In a build without the `relate` tag, the engine-touching leaves must fail
// closed with the typed not-compiled usage error; list and delete (pure
// filesystem work) keep working.

package relate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestRelateCreate_NotCompiledRefusal(t *testing.T) {
	h := newHarness(t, "json")
	err := h.execute(t, "create", "movies")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "not compiled into this build")
}

func TestRelateInfo_NotCompiledRefusal(t *testing.T) {
	h := newHarness(t, "json")
	h.seedStore(t, "movies")

	err := h.execute(t, "info", "movies")
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "not compiled into this build")
}
