// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPayload() Payload {
	p, _, _ := Validate(baseInput(), testRoot())
	return p
}

func TestSend_Classification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		handler    http.HandlerFunc
		wantOut    string
		wantReason string
		wantID     bool // the outcome carries the request id as the report id
	}{
		{"verbose success", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":1,"error":null}`))
		}, "sent", "", true},
		{"bare 1 without verbose", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`1`))
		}, "sent", "", true},
		{"verbose status 0 (no valid event)", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":0,"error":"some error text that must not be surfaced"}`))
		}, "rejected", "server_rejected", false},
		{"bare 0", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`0`)) }, "rejected", "server_rejected", false},
		{"2xx with an empty body", func(w http.ResponseWriter, r *http.Request) {}, "unknown", "no_receipt", false},
		{"2xx with a non-Mixpanel JSON body", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"report_id":"rpt_abc"}`))
		}, "unknown", "no_receipt", false},
		{"2xx with a non-JSON body", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`ok`)) }, "unknown", "no_receipt", false},
		{"400", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) }, "rejected", "server_rejected", false},
		{"401 (wrong token)", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }, "rejected", "server_rejected", false},
		{"413", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(413) }, "rejected", "server_rejected", false},
		{"429 is retryable, so failed", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }, "failed", "server_error", false},
		{"500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }, "failed", "server_error", false},
		{"redirect is not followed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://example.invalid/", http.StatusFound)
		}, "failed", "server_error", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			p := testPayload()
			res := Send(context.Background(), srv.URL, p)
			assert.Equal(t, tc.wantOut, res.Outcome)
			assert.Equal(t, tc.wantReason, res.Reason)
			if tc.wantID {
				assert.Equal(t, p.RequestID, res.ReportID)
			} else {
				assert.Empty(t, res.ReportID)
			}
		})
	}
}

func TestSend_Timeout(t *testing.T) {
	old := sendTimeout
	sendTimeout = 100 * time.Millisecond
	t.Cleanup(func() { sendTimeout = old })

	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(done)

	res := Send(context.Background(), srv.URL, testPayload())
	assert.Equal(t, "unknown", res.Outcome)
	assert.Equal(t, "timeout", res.Reason)
}

func TestSend_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens any more

	res := Send(context.Background(), url, testPayload())
	assert.Equal(t, "failed", res.Outcome)
	assert.Equal(t, "unreachable", res.Reason)
}

func TestSend_RequestShape(t *testing.T) {
	withToken(t, "test-project-token")
	var gotHeaders http.Header
	var gotBody []byte
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"status":1,"error":null}`))
	}))
	defer srv.Close()

	p := testPayload()
	p.CLIVersion = "9.9.9"
	p.CommandID = "query"
	p.ErrorCode = "usage_error"
	p.Agent = &Agent{Harness: "codex", Model: "claude-opus-5-5"}
	p.OS, p.OSArch, p.Invoker = "linux", "arm64", "agent"
	res := Send(context.Background(), srv.URL, p)
	require.Equal(t, "sent", res.Outcome)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/track", gotPath)
	assert.Equal(t, "ip=0&verbose=1", gotQuery, "no IP processing, and a verbose answer")
	// Content-Length is added by net/http for any body; everything else is ours.
	gotHeaders.Del("Content-Length")
	var names []string
	for k := range gotHeaders {
		names = append(names, k)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"Accept", "Content-Type", "User-Agent"}, names)
	assert.Equal(t, "application/json", gotHeaders.Get("Content-Type"))
	assert.Equal(t, "neo4j-cli/9.9.9 feedback", gotHeaders.Get("User-Agent"))

	props := lastProps(t, gotBody)
	assert.Equal(t, map[string]any{
		"token":          "test-project-token",
		"distinct_id":    p.RequestID,
		"$insert_id":     p.RequestID,
		"schema_version": p.SchemaVersion,
		"surface":        p.Surface,
		"cli_version":    "9.9.9",
		"$os":            "linux",
		"os_arch":        "arm64",
		"invoker":        "agent",
		"category":       p.Category,
		"expected":       p.Expected,
		"observed":       p.Observed,
		"command_id":     "query",
		"error_code":     "usage_error",
		"agent_harness":  "codex",
		"agent_model":    "claude-opus-5-5",
	}, props)
	for _, never := range []string{"time", "ip", "$ip", "$device_id", "uptime", "pprocess_binary", "machine_id"} {
		assert.NotContains(t, props, never)
	}
}

func TestSend_OptionalFieldsAreOmittedNotEmpty(t *testing.T) {
	withToken(t, "t")
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`1`))
	}))
	defer srv.Close()
	require.Equal(t, "sent", Send(context.Background(), srv.URL, testPayload()).Outcome)
	props := lastProps(t, body)
	for _, k := range []string{"command_id", "error_code", "agent_harness", "agent_model", "$os", "os_arch", "invoker"} {
		assert.NotContains(t, props, k)
	}
}

func TestSend_BuildWithoutTokenCannotReachANonLoopbackDestination(t *testing.T) {
	withToken(t, "")
	var hits int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer proxy.Close()
	useProxy(t, proxy.URL)
	res := Send(context.Background(), "https://api.mixpanel.com", testPayload())
	assert.Equal(t, "failed", res.Outcome)
	assert.Equal(t, "not_configured", res.Reason)
	assert.Zero(t, hits, "nothing is sent without a token")
}

func TestSend_ElapsedMeasured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":1,"error":null}`))
	}))
	defer srv.Close()
	res := Send(context.Background(), srv.URL, testPayload())
	assert.GreaterOrEqual(t, res.ElapsedMS, int64(30))
	assert.Less(t, res.ElapsedMS, int64(3000))
}

// The endpoint host below does not resolve: a request only reaches the test
// server if it goes through the proxy.
const (
	proxiedEndpoint = "http://feedback.example.test/v1/feedback"
	proxiedTarget   = "http://feedback.example.test/v1/feedback/track?ip=0&verbose=1"
)

func useProxy(t *testing.T, proxyURL string) {
	prevAllowed := sendAllowed
	t.Cleanup(func() { sendAllowed = prevAllowed })
	sendAllowed = func(string) bool { return true } // the test host exists only behind the proxy

	t.Helper()
	u, err := url.Parse(proxyURL)
	require.NoError(t, err)
	old := proxyFunc
	proxyFunc = func(*http.Request) (*url.URL, error) { return u, nil }
	t.Cleanup(func() { proxyFunc = old })
}

func TestSend_UsesProxy(t *testing.T) {
	withToken(t, "t")
	var gotHost, gotURL string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost, gotURL = r.Host, r.URL.String()
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"status":1,"error":null}`))
	}))
	defer proxy.Close()
	useProxy(t, proxy.URL)

	p := testPayload()
	res := Send(context.Background(), proxiedEndpoint, p)
	assert.Equal(t, "sent", res.Outcome)
	assert.Equal(t, p.RequestID, res.ReportID)
	assert.Equal(t, "feedback.example.test", gotHost)
	assert.Equal(t, proxiedTarget, gotURL)
}

func TestSend_ProxyRedirectIsNotFollowed(t *testing.T) {
	withToken(t, "t")
	var hits int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "http://elsewhere.example.test/", http.StatusFound)
	}))
	defer proxy.Close()
	useProxy(t, proxy.URL)

	res := Send(context.Background(), proxiedEndpoint, testPayload())
	assert.Equal(t, "failed", res.Outcome)
	assert.Equal(t, "server_error", res.Reason)
	assert.Equal(t, 1, hits, "exactly one request, no follow-up")
}

func TestSend_UnreachableProxyFailsWithoutFallingBackToDirect(t *testing.T) {
	withToken(t, "t")
	var direct int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct++ }))
	defer target.Close()
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	useProxy(t, deadURL)

	res := Send(context.Background(), proxiedEndpoint, testPayload())
	assert.Equal(t, "failed", res.Outcome)
	assert.Equal(t, "unreachable", res.Reason)
	assert.Zero(t, direct)
}

func TestSend_LoopbackIsNeverProxiedByDefault(t *testing.T) {
	var proxied int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxied++ }))
	defer proxy.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"status":1,"error":null}`))
	}))
	defer target.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)

	req, err := http.NewRequest(http.MethodPost, target.URL, nil)
	require.NoError(t, err)
	got, err := http.ProxyFromEnvironment(req)
	require.NoError(t, err)
	assert.Nil(t, got, "the standard library skips the proxy for loopback")

	res := Send(context.Background(), target.URL, testPayload())
	assert.Equal(t, "sent", res.Outcome)
	assert.Zero(t, proxied)
}

func TestSend_RefusesADestinationTheAllowListRefuses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	res := Send(context.Background(), srv.URL+"/x#frag", testPayload())
	assert.Equal(t, "failed", res.Outcome)
	assert.Equal(t, "endpoint_not_allowed", res.Reason)
	assert.EqualValues(t, 0, hits.Load())
}

func TestSend_CancelledMidRequestIsUnknownNotFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel() // the person cancels after the server has the report
		time.Sleep(50 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)
	res := Send(ctx, srv.URL+"/v1/feedback", testPayload())
	assert.Equal(t, "unknown", res.Outcome)
	assert.Equal(t, "cancelled", res.Reason)
}
