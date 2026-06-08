// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostPortOf returns the host + port of a httptest server URL so probe seams
// can be pointed at it.
func hostPortOf(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return u.Hostname(), port
}

func TestProbeOne_DebugTracesProbeTargetAndStatus(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == ProbePath {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	host, port := hostPortOf(t, srv.URL)
	t.Cleanup(SetProbeHostFnForTest(func() string { return host }))
	t.Cleanup(SetHTTPClientFnForTest(func() *http.Client { return srv.Client() }))

	res, err := ProbePort(context.Background(), port)
	require.NoError(t, err)
	assert.Equal(t, port, res.Port)

	out := buf.String()
	assert.Contains(t, out, "[desktop-debug] ")
	assert.Contains(t, out, "probe GET")
	assert.Contains(t, out, ProbePath)
	assert.Contains(t, out, "-> 200")
}

func TestDiscover_DebugTracesMDNSAndResolvedTier(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	// Pin mDNS to answer with a port so Discover resolves via the mDNS tier
	// without touching the real network or the port scan.
	t.Cleanup(SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 44225, true }))

	res, err := Discover(context.Background(), 0)
	require.NoError(t, err)
	assert.Contains(t, res.Origin, "127.0.0.1:44225")

	out := buf.String()
	assert.Contains(t, out, "mdns multicast advertised port 44225")
	assert.Contains(t, out, "discover: resolved via mdns origin")
}

func TestFetchAppInfo_DebugTracesRequestAndResponse(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, infoAppPath, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"dataPath":"/tmp/data","version":"2.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	info, err := FetchAppInfo(context.Background(), ProbeResult{Origin: srv.URL})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/data", info.DataPath)

	out := buf.String()
	assert.Contains(t, out, "[desktop-debug] > GET")
	assert.Contains(t, out, infoAppPath)
	assert.Contains(t, out, "[desktop-debug] < 200")
	assert.Contains(t, out, "elapsed")
}

func TestDiscovery_OffPathEmitsNothing(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)
	SetDebugForTest(t, false)

	t.Cleanup(SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) { return 44225, true }))

	_, err := Discover(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
