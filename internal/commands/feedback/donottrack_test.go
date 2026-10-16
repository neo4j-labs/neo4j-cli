// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoNotTrack_EnableRefuses(t *testing.T) {
	withSeams(t, false, "")
	cfg := newTestConfig(t)
	t.Setenv("DO_NOT_TRACK", "1")
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback)
	assert.Equal(t, 2, exitCode(err))
	assert.Empty(t, stdout, "no disclosure is printed")
	assert.False(t, Authorized(cfg, now()), "a refused enable must not authorize")
	assert.Empty(t, endpoint(cfg), "a refused enable must not store the endpoint")
}

func TestDoNotTrack_OnlyOneCountsAsSet(t *testing.T) {
	withSeams(t, false, "")
	for _, v := range []string{"", "0", "true"} {
		cfg := newTestConfig(t)
		t.Setenv("DO_NOT_TRACK", v)
		_, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback, "--register=false")
		require.NoError(t, err, "DO_NOT_TRACK=%q", v)
	}
}

func TestDoNotTrack_SubmitSkipsEvenWhenEnabled(t *testing.T) {
	withSeams(t, false, "")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)
	t.Setenv("DO_NOT_TRACK", "1")

	stdout, _, err := runCmd(t, newTestApp(cfg), with()...)
	require.NoError(t, err)
	out := parseJSON(t, stdout)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "do_not_track", out["reason"])
	assert.Empty(t, out["receipt_path"], "nothing is recorded")
	assert.EqualValues(t, 0, hits.Load())
}

func TestDoNotTrack_ToolSkipsWithoutAsking(t *testing.T) {
	withSeams(t, false, "claude-code")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	for _, enabled := range []bool{true, false} {
		cs := connectToolClient(t, srv, enabled) // resets DO_NOT_TRACK, so set it afterwards
		t.Setenv("DO_NOT_TRACK", "1")
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "feedback_submit",
			Arguments: map[string]any{"category": "unclear_error", "expected": baseExpected, "observed": baseObserved}})
		require.NoError(t, err)
		assert.False(t, res.IsError)
		out := toolOutcome(t, res)
		assert.Equal(t, "skipped", out["outcome"], "enabled=%v", enabled)
		assert.Equal(t, "do_not_track", out["reason"])
	}
	assert.EqualValues(t, 0, hits.Load())
}

func TestDoNotTrack_StatusShowsOverrideAndDisableStillWorks(t *testing.T) {
	withSeams(t, false, "")
	cfg := newTestConfig(t)
	enableFor(t, cfg, loopback)
	t.Setenv("DO_NOT_TRACK", "1")

	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "status")
	require.NoError(t, err)
	st := parseJSON(t, stdout)
	assert.Equal(t, false, st["enabled"])
	assert.Equal(t, true, st["do_not_track"])

	_, _, err = runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	assert.False(t, Authorized(cfg, now()))
}

func TestStatus_DoNotTrackFalseByDefault(t *testing.T) {
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "status")
	require.NoError(t, err)
	assert.Equal(t, false, parseJSON(t, stdout)["do_not_track"])
}

// telemetryOffConfig returns a test config whose CLI telemetry is turned off,
// as `neo4j-cli config set telemetry false` stores it.
func telemetryOffConfig(t *testing.T) *clicfg.Config {
	t.Helper()
	cfg := newTestConfig(t)
	require.NoError(t, cfg.Global.Set("telemetry", "false"))
	return cfg
}

func TestTelemetryOff_EnableRefusesBeforeTheAgentCheck(t *testing.T) {
	withSeams(t, true, "claude-code") // even under an agent, the opt-out is what is reported
	cfg := telemetryOffConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback)
	assert.Equal(t, 2, exitCode(err))
	assert.Empty(t, stdout, "no disclosure is printed")
	assert.False(t, Authorized(cfg, now()))
}

func TestTelemetryOff_SubmitSkipsEvenWhenEnabled(t *testing.T) {
	withSeams(t, false, "")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)
	require.NoError(t, cfg.Global.Set("telemetry", "false"))

	stdout, _, err := runCmd(t, newTestApp(cfg), with()...)
	require.NoError(t, err)
	out := parseJSON(t, stdout)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "telemetry_off", out["reason"])
	assert.EqualValues(t, 0, hits.Load())
}

func TestTelemetryOff_ToolSkipsWithoutAsking(t *testing.T) {
	withSeams(t, false, "claude-code")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	for _, enabled := range []bool{true, false} {
		cs, cfg := connectToolClientWithConfig(t, srv, enabled)
		require.NoError(t, cfg.Global.Set("telemetry", "false"))
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "feedback_submit",
			Arguments: map[string]any{"category": "unclear_error", "expected": baseExpected, "observed": baseObserved}})
		require.NoError(t, err)
		out := toolOutcome(t, res)
		assert.Equal(t, "skipped", out["outcome"], "enabled=%v", enabled)
		assert.Equal(t, "telemetry_off", out["reason"])
	}
	assert.EqualValues(t, 0, hits.Load())
}

func TestTelemetryOff_StatusShowsIt(t *testing.T) {
	cfg := newTestConfig(t)
	enableFor(t, cfg, loopback)
	require.NoError(t, cfg.Global.Set("telemetry", "false"))
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "status")
	require.NoError(t, err)
	st := parseJSON(t, stdout)
	assert.Equal(t, false, st["enabled"])
	assert.Equal(t, true, st["telemetry_off"])
}
