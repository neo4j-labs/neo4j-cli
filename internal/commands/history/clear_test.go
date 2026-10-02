// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClear_RequiresForce(t *testing.T) {
	cfg := newTestConfigFmt(t, "table")
	seedEntries(t, cfg, []Entry{mkEntry("neo4j-cli instance list", "agent", time.Now().UTC())})

	out, err := runCmd(t, newClearCmd(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")

	// File untouched.
	entries, loadErr := Load(cfg)
	require.NoError(t, loadErr)
	assert.Len(t, entries, 1)
	_ = out
}

func TestClear_WithForceEmptiesFile(t *testing.T) {
	cfg := newTestConfigFmt(t, "table")
	seedEntries(t, cfg, []Entry{
		mkEntry("neo4j-cli instance list", "agent", time.Now().UTC()),
		mkEntry("neo4j-cli config list", "agent", time.Now().UTC()),
	})

	out, err := runCmd(t, newClearCmd(cfg), "--force")
	require.NoError(t, err)
	assert.Contains(t, out, "History cleared.")

	entries, loadErr := Load(cfg)
	require.NoError(t, loadErr)
	assert.Empty(t, entries)
}
