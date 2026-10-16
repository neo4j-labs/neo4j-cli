// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisable_RevokesAndUnregisters(t *testing.T) {
	withSeams(t, false, "")
	calls := fakeClaude(t, true, false)
	cfg := newTestConfig(t)
	enableFor(t, cfg, loopback)

	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	assert.Equal(t, false, parseJSON(t, stdout)["enabled"])
	assert.False(t, Authorized(cfg, now()))
	assert.Contains(t, *calls, "mcp remove -s user neo4j-feedback")
}

// Revoking is never gated: an agent asked to turn feedback off can do it
// without --rw, unlike enable and register.
func TestDisable_NeedsNoRWUnderAnAgent(t *testing.T) {
	t.Setenv("CLAUDECODE", "1") // what the root write gate reads
	withSeams(t, true, "claude-code")
	fakeClaude(t, false, false)
	cfg := newTestConfig(t)
	enableFor(t, cfg, loopback)

	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	assert.False(t, Authorized(cfg, now()))
}
