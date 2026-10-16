// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const loopback = "http://127.0.0.1:8765/v1/feedback"

func TestEnable_RefusalPaths(t *testing.T) {
	for _, tc := range []struct {
		name     string
		agent    bool
		args     []string
		wantExit int
		wantOut  string
	}{
		{"agent detected", true, []string{"feedback", "enable", "--rw", "--accept", "--endpoint", loopback}, 4, ""},
		{"without --accept prints disclosure", false, []string{"feedback", "enable", "--rw", "--endpoint", loopback}, 2, "Re-run with --accept"},
		{"non-loopback endpoint", false, []string{"feedback", "enable", "--rw", "--accept", "--endpoint", "http://example.com/x"}, 2, ""},
		{"https endpoint", false, []string{"feedback", "enable", "--rw", "--accept", "--endpoint", "https://127.0.0.1/x"}, 2, ""},
		{"missing endpoint", false, []string{"feedback", "enable", "--rw", "--accept"}, 2, ""},
		{"days 0", false, []string{"feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--days", "0"}, 2, ""},
		{"days 91", false, []string{"feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--days", "91"}, 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSeams(t, tc.agent, "")
			cfg := newTestConfig(t)
			stdout, _, err := runCmd(t, newTestApp(cfg), tc.args...)
			assert.Equal(t, tc.wantExit, exitCode(err))
			if tc.wantOut != "" {
				assert.Contains(t, stdout, tc.wantOut)
				assert.Contains(t, stdout, loopback, "disclosure names the destination")
				assert.Contains(t, stdout, "expires on")
			}
			assert.False(t, Authorized(cfg, now()), "a refused enable must not authorize")
			assert.Empty(t, endpoint(cfg), "a refused enable must not store the endpoint")
		})
	}
}

func TestEnable_Success(t *testing.T) {
	withSeams(t, false, "")
	cfg := newTestConfig(t)
	before := time.Now()
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--days", "7")
	require.NoError(t, err)

	st := parseJSON(t, stdout)
	assert.Equal(t, true, st["enabled"])
	assert.Equal(t, loopback, st["endpoint"])
	assert.NotEmpty(t, st["receipts_path"])

	until, ok := authorizedUntil(cfg)
	require.True(t, ok)
	assert.WithinDuration(t, before.Add(7*24*time.Hour), until, time.Minute)
}

func TestEnable_KeysNotConfigSettable(t *testing.T) {
	cfg := newTestConfig(t)
	for _, k := range []string{keyAuthorizedUntil, keyEndpoint} {
		assert.False(t, cfg.Global.IsValidConfigKey(k), "%s must not be settable via `config set`", k)
	}
}

func TestEnable_UsesTheBuildDestinationWhenNoEndpointIsGiven(t *testing.T) {
	withSeams(t, false, "")
	withToken(t, "some-token")
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--register=false")
	require.NoError(t, err)
	assert.Equal(t, mixpanelBase, parseJSON(t, stdout)["endpoint"])
	assert.Equal(t, mixpanelBase, endpoint(cfg))
}

func TestEnable_BuildWithoutADestinationNeedsAnEndpoint(t *testing.T) {
	withSeams(t, false, "")
	withToken(t, "")
	cfg := newTestConfig(t)
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--register=false")
	assert.Equal(t, 2, exitCode(err))
	assert.False(t, Authorized(cfg, now()))
	assert.Empty(t, endpoint(cfg))
}

func TestEnable_AcceptNeedsAPersonAtATerminal(t *testing.T) {
	withSeams(t, false, "")
	asScript(t) // e.g. an agent harness this CLI does not recognize, or a pipe
	cfg := newTestConfig(t)
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--register=false")
	assert.Equal(t, 4, exitCode(err))
	assert.False(t, Authorized(cfg, now()))

	// Showing the disclosure (no --accept) still works without a terminal.
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--endpoint", loopback)
	assert.Equal(t, 2, exitCode(err))
	assert.Contains(t, stdout, "Enabling neo4j-cli feedback")
}

func TestAuthorized_AnExpiryBeyondWhatEnableCanSetIsIgnored(t *testing.T) {
	cfg := newTestConfig(t)
	require.NoError(t, cfg.Global.Set(keyAuthorizedUntil, "2999-01-01T00:00:00Z"))
	assert.False(t, Authorized(cfg, now()), "a hand-edited far-future expiry is not an authorization")
	require.NoError(t, cfg.Global.Set(keyAuthorizedUntil, now().Add(maxDays*24*time.Hour).UTC().Format(time.RFC3339)))
	assert.True(t, Authorized(cfg, now()), "the longest expiry enable can set is accepted")
}
