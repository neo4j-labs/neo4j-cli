// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func connectToolClient(t *testing.T, cfgSrv *httptest.Server, enabled bool) *mcpsdk.ClientSession {
	t.Helper()
	cs, _ := connectToolClientWithConfig(t, cfgSrv, enabled)
	return cs
}

// connectToolClientWithConfig is connectToolClient that also returns the
// server's config, so a test can change settings while the server runs.
func connectToolClientWithConfig(t *testing.T, cfgSrv *httptest.Server, enabled bool) (*mcpsdk.ClientSession, *clicfg.Config) {
	t.Helper()
	cfg := newTestConfig(t)
	if enabled {
		enableFor(t, cfg, cfgSrv.URL)
	}
	srv := newFeedbackServer(cfg, newTestApp(cfg))
	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ss, err := srv.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs, cfg
}

func toolOutcome(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	require.Len(t, res.Content, 1)
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	require.True(t, ok)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &m), tc.Text)
	return m
}

func TestMCP_ListsExactlyOneToolWithSchemaAndDescription(t *testing.T) {
	withSeams(t, false, "")
	cs := connectToolClient(t, nil, false)
	list, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 1)
	tool := list.Tools[0]
	assert.Equal(t, "feedback_submit", tool.Name)
	assert.Equal(t, feedbackToolDescription, tool.Description)
	assert.True(t, strings.HasPrefix(tool.Description, "Report a problem with neo4j-cli itself to the Neo4j team."))
	b, _ := json.Marshal(tool.InputSchema)
	s := string(b)
	for _, want := range []string{`"maxLength":255`, `"required":["category","expected","observed"]`, `"unclear_error"`, `"command"`} {
		assert.Contains(t, s, want)
	}
}

func TestMCP_SubmitSentRejectedAndSkipped(t *testing.T) {
	withSeams(t, false, "codex")
	var hits atomic.Int32
	var last []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		last = buf[:n]
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"status":1,"error":null}`))
	}))
	t.Cleanup(srv.Close)
	cs := connectToolClient(t, srv, true)
	call := func(args map[string]any) *mcpsdk.CallToolResult {
		res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "feedback_submit", Arguments: args})
		require.NoError(t, err)
		return res
	}

	res := call(map[string]any{"category": "missing_capability", "expected": baseExpected, "observed": baseObserved, "command": "query"})
	assert.False(t, res.IsError)
	out := toolOutcome(t, res)
	assert.Equal(t, "sent", out["outcome"])
	assert.Equal(t, out["request_id"], out["report_id"])
	assert.EqualValues(t, 1, hits.Load())
	assert.Equal(t, "codex", lastProps(t, last)["agent_harness"])

	res = call(map[string]any{"category": "missing_capability", "expected": strings.Repeat("a", 256), "observed": baseObserved})
	assert.True(t, res.IsError)
	out = toolOutcome(t, res)
	assert.Equal(t, "rejected", out["outcome"])
	assert.EqualValues(t, 1, hits.Load(), "a rejected report never reaches the network")

	res = call(map[string]any{"category": "unclear_error", "expected": "The message did not say which user table is missing", "observed": "Contact me at ada@example.com"})
	assert.Equal(t, "rejected", toolOutcome(t, res)["outcome"])
	assert.EqualValues(t, 1, hits.Load())

	disabled := connectToolClient(t, srv, false)
	res, err := disabled.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "feedback_submit",
		Arguments: map[string]any{"category": "unclear_error", "expected": baseExpected, "observed": baseObserved}})
	require.NoError(t, err)
	out = toolOutcome(t, res)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "not_enabled", out["reason"])
	assert.EqualValues(t, 1, hits.Load())
}

func TestMCPCommand_IsHiddenWithExample(t *testing.T) {
	cmd := NewCmd(newTestConfig(t))
	for _, c := range cmd.Commands() {
		if c.Name() == "serve" {
			assert.True(t, c.Hidden)
			assert.NotEmpty(t, c.Example)
			return
		}
	}
	t.Fatal("feedback serve not registered")
}
