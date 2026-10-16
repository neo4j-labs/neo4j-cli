// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister_DoesNotAuthorizeAndIsAllowedUnderAgent(t *testing.T) {
	withSeams(t, true, "claude-code")
	calls := fakeClaude(t, true, false)
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw", "--endpoint", loopback)
	require.NoError(t, err)
	st := parseJSON(t, stdout)
	assert.Equal(t, false, st["enabled"], "register authorizes nothing")
	assert.Equal(t, []any{"claude-code"}, st["registered_agents"])
	assert.Contains(t, *calls, "mcp add -s user neo4j-feedback -e NEO4J_CLI_FEEDBACK_INSTRUCTIONS=basic -e NEO4J_CLI_FLAG_FEEDBACK_COMMAND=true -- /opt/neo4j-cli feedback serve")
	assert.False(t, Authorized(cfg, time.Now()))
	assert.Equal(t, loopback, endpoint(cfg))
}

func TestRegisterCmd_CannotRedirectAnExistingAuthorization(t *testing.T) {
	withSeams(t, true, "claude-code")
	fakeClaude(t, true, false)
	cfg := newTestConfig(t)
	enableFor(t, cfg, loopback)

	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw", "--endpoint", "http://localhost:9999/other")
	assert.Equal(t, 5, exitCode(err))
	assert.Equal(t, loopback, endpoint(cfg), "the authorized destination is unchanged")

	// Without --endpoint, re-registering keeps the authorized destination.
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw")
	require.NoError(t, err)
	assert.Equal(t, loopback, endpoint(cfg))
	assert.NotContains(t, stdout, "Not enabled yet")
}

func TestRegisterCmd_InstructionsVariantIsAPersonsChoice(t *testing.T) {
	withSeams(t, true, "claude-code")
	fakeClaude(t, true, false)
	cfg := newTestConfig(t)
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw", "--endpoint", loopback, "--instructions", "optin")
	assert.Equal(t, 4, exitCode(err))
	assert.Empty(t, getString(cfg, keyInstructions))
}

func TestRegisterAndEnable_AreWritesThatNeedRWUnderAnAgent(t *testing.T) {
	t.Setenv("CLAUDECODE", "1") // what the root write gate reads
	withSeams(t, true, "claude-code")
	calls := fakeClaude(t, false, false)
	cfg := newTestConfig(t)
	for _, args := range [][]string{
		{"feedback", "register", "--endpoint", loopback},
		{"feedback", "enable", "--endpoint", loopback},
	} {
		_, _, err := runCmd(t, newTestApp(cfg), args...)
		assert.Equal(t, 2, exitCode(err), "%v", args)
		assert.ErrorContains(t, err, "pass --rw", "%v", args)
	}
	assert.Empty(t, *calls, "nothing was registered")
	assert.Empty(t, endpoint(cfg), "nothing was stored")
}

func TestStatus_FollowsFormat(t *testing.T) {
	withSeams(t, false, "")
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "status", "--format", "table")
	require.NoError(t, err)
	assert.Contains(t, stdout, "enabled")
	assert.Contains(t, stdout, "receipts_path")
	assert.NotContains(t, stdout, "{", "a table, not JSON")

	stdout, _, err = runCmd(t, newTestApp(cfg), "feedback", "status", "--format", "toon")
	require.NoError(t, err)
	assert.Contains(t, stdout, "enabled: false")
}
