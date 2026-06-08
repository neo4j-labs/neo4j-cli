// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pollTestServer serves /v1/instances/abc returning "creating" until the
// requested attempt, then "running".
func pollTestServer(t *testing.T, readyAfter int32) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/instances/abc", func(w http.ResponseWriter, r *http.Request) {
		status := "creating"
		if calls.Add(1) >= readyAfter {
			status = "running"
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"id":"abc","status":"` + status + `"}}`)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestPoll_DebugEmitsLoopContext(t *testing.T) {
	srv := pollTestServer(t, 2)
	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	cfg.Aura.SetDebug(true)
	cfg.Aura.SetPollingConfig(5, 0)

	res, err := api.PollInstance(cfg, "abc", "creating")
	require.NoError(t, err)
	assert.Equal(t, "running", res.Data.Status)

	out := buf.String()
	// Loop-level context: attempt index/max, polled path, status, interval.
	assert.Contains(t, out, "poll attempt 1/5")
	assert.Contains(t, out, "poll attempt 2/5")
	assert.Contains(t, out, "/instances/abc")
	assert.Contains(t, out, "status 200")
	assert.Contains(t, out, `observed "creating"`)
	assert.Contains(t, out, `observed "running"`)
	assert.Contains(t, out, "interval 0s")
}

func TestPoll_DebugOffEmitsNoLoopLines(t *testing.T) {
	srv := pollTestServer(t, 1)
	var buf bytes.Buffer
	api.SetDebugWriterForTest(t, &buf)

	cfg := buildTestConfig(t, srv.URL, debugTestCredJSON)
	// debug not set -> off
	cfg.Aura.SetPollingConfig(5, 0)

	_, err := api.PollInstance(cfg, "abc", "creating")
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "poll attempt")
}
