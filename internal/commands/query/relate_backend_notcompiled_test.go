// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !relate

// In a build without the `relate` tag, --relate must fail closed with the
// typed not-compiled usage error — never a nil-hook panic and never a silent
// fallback to the default Bolt connection.

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func TestRunQueryRelate_NotCompiledRefusal(t *testing.T) {
	h := newRunHarness(t, "json")
	err := h.execute(t,
		"--relate=/tmp/store",
		"RETURN 1 AS one",
	)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, err.Error(), "not compiled into this build")
}
