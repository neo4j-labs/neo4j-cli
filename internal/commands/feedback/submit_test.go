// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validArgs = []string{
	"feedback", "submit",
	"--category", "unexpected_behavior",
	"--expected", baseExpected,
	"--observed", baseObserved,
}

func with(extra ...string) []string { return append(append([]string{}, validArgs...), extra...) }

// counting returns a server that counts hits and records the last body.
func counting(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32, *[]byte) {
	t.Helper()
	var hits atomic.Int32
	var last []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		last, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &last
}

func TestSubmit_OutcomeTable(t *testing.T) {
	withSeams(t, false, "")
	old := sendTimeout
	t.Cleanup(func() { sendTimeout = old })

	for _, tc := range []struct {
		name       string
		setup      func(t *testing.T) (endpoint string, hits *atomic.Int32) // nil: not enabled
		args       []string
		wantOut    string
		wantReason string
		wantExit   int
		wantHits   int32
		wantNoPay  bool // receipt payload must be null
	}{
		{
			name: "sent",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 202, `{"status":1,"error":null}`)
				return srv.URL, h
			},
			args: validArgs, wantOut: "sent", wantExit: 0, wantHits: 1,
		},
		{name: "skipped not_enabled", args: validArgs, wantOut: "skipped", wantReason: "not_enabled", wantExit: 0},
		{
			name: "skipped authorization_expired",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 202, `{"status":1,"error":null}`)
				return srv.URL + "#expired", h
			},
			args: validArgs, wantOut: "skipped", wantReason: "authorization_expired", wantExit: 0,
		},
		{
			name: "rejected invalid_input",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 202, `{"status":1,"error":null}`)
				return srv.URL, h
			},
			args:    []string{"feedback", "submit", "--category", "praise", "--expected", baseExpected, "--observed", baseObserved},
			wantOut: "rejected", wantReason: "invalid_input", wantExit: 6, wantNoPay: true,
		},
		{
			name: "rejected policy_violation",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 202, `{"status":1,"error":null}`)
				return srv.URL, h
			},
			args:    []string{"feedback", "submit", "--category", "unclear_error", "--expected", baseExpected, "--observed", "Owner was z.okonkwo@canary-example.test"},
			wantOut: "rejected", wantReason: "policy_violation", wantExit: 6, wantNoPay: true,
		},
		{
			name: "rejected endpoint_not_allowed",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				return "http://example.com/x", &atomic.Int32{}
			},
			args:    validArgs,
			wantOut: "rejected", wantReason: "endpoint_not_allowed", wantExit: 6, wantNoPay: true,
		},
		{
			name: "rejected server_rejected",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 400, `{"status":"rejected"}`)
				return srv.URL, h
			},
			args: validArgs, wantOut: "rejected", wantReason: "server_rejected", wantExit: 6, wantHits: 1,
		},
		{
			name: "failed unreachable",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
				u := srv.URL
				srv.Close()
				return u, &atomic.Int32{}
			},
			args: validArgs, wantOut: "failed", wantReason: "unreachable", wantExit: 8,
		},
		{
			name: "failed server_error",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 500, ``)
				return srv.URL, h
			},
			args: validArgs, wantOut: "failed", wantReason: "server_error", wantExit: 8, wantHits: 1,
		},
		{
			name: "unknown timeout",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				sendTimeout = 100 * time.Millisecond
				done := make(chan struct{})
				var hits atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					select {
					case <-done:
					case <-time.After(2 * time.Second):
					}
				}))
				t.Cleanup(func() { close(done); srv.Close() })
				return srv.URL, &hits
			},
			args: validArgs, wantOut: "unknown", wantReason: "timeout", wantExit: 8, wantHits: 1,
		},
		{
			name: "unknown no_receipt",
			setup: func(t *testing.T) (string, *atomic.Int32) {
				srv, h, _ := counting(t, 202, `{"status":"accepted"}`)
				return srv.URL, h
			},
			args: validArgs, wantOut: "unknown", wantReason: "no_receipt", wantExit: 8, wantHits: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sendTimeout = old
			cfg := newTestConfig(t)
			var hits *atomic.Int32
			if tc.setup != nil {
				var ep string
				ep, hits = tc.setup(t)
				enableFor(t, cfg, ep)
				if tc.wantReason == "authorization_expired" {
					require.NoError(t, cfg.Global.Set(keyEndpoint, ep[:len(ep)-len("#expired")]))
					require.NoError(t, cfg.Global.Set(keyAuthorizedUntil, now().Add(-time.Hour).UTC().Format(time.RFC3339)))
				}
			}

			stdout, _, err := runCmd(t, newTestApp(cfg), tc.args...)
			assert.Equal(t, tc.wantExit, exitCode(err))

			out := parseJSON(t, stdout)
			assert.Equal(t, tc.wantOut, out["outcome"])
			assert.Equal(t, tc.wantReason, out["reason"])
			if hits != nil {
				assert.Equal(t, tc.wantHits, hits.Load(), "request count")
			}

			if tc.wantOut == "sent" {
				assert.Equal(t, out["request_id"], out["report_id"], "Mixpanel returns no id; the request id stands in")
				assert.NotNil(t, out["payload"])
				assert.LessOrEqual(t, out["elapsed_ms"], float64(3000))
			} else {
				assert.Nil(t, out["payload"], "payload is echoed only when sent")
			}

			if tc.wantOut == "skipped" {
				assert.Empty(t, out["receipt_path"], "skips write no receipt")
				return
			}
			lines := readReceipts(t, cfg.Fs(), receiptsPath())
			require.Len(t, lines, 1)
			assert.Equal(t, tc.wantOut, lines[0]["outcome"])
			if tc.wantNoPay {
				assert.Nil(t, lines[0]["payload"])
			} else {
				assert.NotNil(t, lines[0]["payload"])
			}
		})
	}
}

func TestSubmit_RejectionNeverEchoesMatchedText(t *testing.T) {
	withSeams(t, false, "")
	srv, hits, _ := counting(t, 202, `{"status":1,"error":null}`)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)

	secret := "password=Hunter2Hunter2"
	stdout, stderr, err := runCmd(t, newTestApp(cfg), "feedback", "submit", "--category", "unclear_error",
		"--expected", baseExpected, "--observed", "It failed even with "+secret)
	assert.Equal(t, 6, exitCode(err))
	assert.NotContains(t, stdout, "Hunter2")
	assert.NotContains(t, stderr, "Hunter2")
	assert.NotContains(t, err.Error(), "Hunter2")
	assert.EqualValues(t, 0, hits.Load())

	lines := readReceipts(t, cfg.Fs(), receiptsPath())
	require.Len(t, lines, 1)
	b, _ := json.Marshal(lines[0])
	assert.NotContains(t, string(b), "Hunter2")

	out := parseJSON(t, stdout)
	vs := out["violations"].([]any)
	require.Len(t, vs, 1)
	assert.Equal(t, map[string]any{"field": "observed", "detector": "secret"}, vs[0])
}

func TestSubmit_DroppedFieldsAndHarness(t *testing.T) {
	withSeams(t, false, "codex")
	srv, _, last := counting(t, 202, `{"status":1,"error":null}`)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)

	stdout, _, err := runCmd(t, newTestApp(cfg), with("--command", "query --uri x", "--agent-model", "MyCustomBot", "--error-code", "usage_error")...)
	require.NoError(t, err)
	out := parseJSON(t, stdout)
	assert.ElementsMatch(t, []any{"command_id", "agent.model"}, out["dropped_fields"])

	sent := lastProps(t, *last)
	assert.NotContains(t, sent, "command_id")
	assert.NotContains(t, sent, "agent_model", "a dropped field is not sent")
	assert.Equal(t, "usage_error", sent["error_code"])
	assert.Equal(t, "codex", sent["agent_harness"])
	assert.Equal(t, "test-version", sent["cli_version"])
	assert.Equal(t, "cli", sent["surface"])
	assert.Equal(t, runtime.GOOS, sent["$os"])
	assert.Equal(t, runtime.GOARCH, sent["os_arch"])
	assert.Equal(t, "human", sent["invoker"], "the seam reports a person when no agent is detected")
	assert.Equal(t, out["payload"].(map[string]any)["os"], sent["$os"], "the receipt payload shows what was sent")
}

func TestSubmit_CommandAllowListAndRequestID(t *testing.T) {
	withSeams(t, false, "")
	srv, _, last := counting(t, 202, `{"status":1,"error":null}`)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)

	rid := "22222222-2222-4222-8222-222222222222"
	stdout, _, err := runCmd(t, newTestApp(cfg), with("--command", "query", "--request-id", rid)...)
	require.NoError(t, err)
	out := parseJSON(t, stdout)
	assert.Equal(t, rid, out["request_id"])
	assert.Empty(t, out["dropped_fields"])

	sent := lastProps(t, *last)
	assert.Equal(t, "query", sent["command_id"])
	assert.Equal(t, rid, sent["distinct_id"], "one report is one distinct_id")
	assert.Equal(t, rid, sent["$insert_id"], "the request id deduplicates a deliberate retry")
}

func TestSubmit_StdoutIsOneDocumentWhateverFormat(t *testing.T) {
	withSeams(t, false, "")
	srv, _, _ := counting(t, 202, `{"status":1,"error":null}`)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)

	stdout, _, err := runCmd(t, newTestApp(cfg), with("--format", "table")...)
	require.NoError(t, err)
	assert.Equal(t, "sent", parseJSON(t, stdout)["outcome"])
}

func TestSubmit_HelpText(t *testing.T) {
	cmd := newSubmitCmd(newTestConfig(t))
	assert.Equal(t, submitShort, cmd.Short)
	assert.Contains(t, cmd.Long, dataPolicyBlock)
	assert.Equal(t, groupShort, NewCmd(newTestConfig(t)).Short)
}

func TestSubmit_TooLongIsRejectedWithoutNetwork(t *testing.T) {
	withSeams(t, false, "")
	srv, hits, _ := counting(t, 202, `{"status":1,"error":null}`)
	cfg := newTestConfig(t)
	enableFor(t, cfg, srv.URL)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "submit", "--category", "unexpected_behavior",
		"--expected", strings.Repeat("a", 256), "--observed", baseObserved)
	assert.Equal(t, 6, exitCode(err))
	out := parseJSON(t, stdout)
	assert.Equal(t, "rejected", out["outcome"])
	assert.Equal(t, "invalid_input", out["reason"])
	assert.EqualValues(t, 0, hits.Load())
	assert.NotContains(t, stdout, strings.Repeat("a", 40), "rejected text is never echoed")
}
