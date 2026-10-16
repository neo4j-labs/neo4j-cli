// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

const evalRunCollectionPath = "/v2beta1/organizations/org-1/projects/proj-1/agents/evaluation-runs"

var evalRunScope = Scope{OrgID: "org-1", ProjectID: "proj-1"}

// evalRunRequest is one request the mock recorded.
type evalRunRequest struct {
	method string
	path   string
	query  url.Values
	body   map[string]any
}

// evalRunMock is a mock Aura API for the evaluation-run endpoints: it serves
// the token endpoint, records every API request, and answers with whatever the
// test's respond function writes.
type evalRunMock struct {
	srv *httptest.Server

	mu       sync.Mutex
	requests []evalRunRequest
	respond  func(w http.ResponseWriter, r *http.Request)
}

func newEvalRunMock(t *testing.T) *evalRunMock {
	t.Helper()
	m := &evalRunMock{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		req := evalRunRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			var body map[string]any
			if json.Unmarshal(raw, &body) == nil {
				req.body = body
			}
		}
		m.mu.Lock()
		m.requests = append(m.requests, req)
		respond := m.respond
		m.mu.Unlock()
		if respond == nil {
			http.NotFound(w, r)
			return
		}
		respond(w, r)
	})
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func (m *evalRunMock) recorded() []evalRunRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]evalRunRequest(nil), m.requests...)
}

// client wires a Client (and its config, for polling overrides) to the mock.
func (m *evalRunMock) client(t *testing.T) (Client, *clicfg.Config) {
	t.Helper()
	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, m.srv.URL, m.srv.URL)
	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")
	return New(cfg), cfg
}

func (m *evalRunMock) writeJSON(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	w.Write([]byte(body)) //nolint:errcheck
}

func TestEvaluationRunCreate_Accepts202And200(t *testing.T) {
	for _, status := range []int{http.StatusAccepted, http.StatusOK} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			m := newEvalRunMock(t)
			m.respond = func(w http.ResponseWriter, _ *http.Request) {
				m.writeJSON(w, status, `{"data":{"id":"run-1","agent_id":"agent-1","dataset_id":"ds-1","status":"queued"},"links":{"resource":"/x"}}`)
			}
			c, _ := m.client(t)

			run, err := c.EvaluationRuns().Create(context.Background(), evalRunScope, EvaluationRunSpec{
				AgentID:   "agent-1",
				DatasetID: "ds-1",
				Metrics:   []string{"accuracy", "tool_trajectory"},
			})

			require.NoError(t, err)
			assert.Equal(t, "run-1", run.ID)
			assert.Equal(t, "agent-1", run.AgentID)
			assert.Equal(t, "ds-1", run.DatasetID)
			assert.Equal(t, "queued", run.Status)
			assert.Equal(t, "queued", run.Record["status"], "Record carries the server record")

			reqs := m.recorded()
			require.Len(t, reqs, 1)
			assert.Equal(t, http.MethodPost, reqs[0].method)
			assert.Equal(t, evalRunCollectionPath, reqs[0].path)
			assert.Equal(t, map[string]any{
				"agent_id":   "agent-1",
				"dataset_id": "ds-1",
				"metrics":    []any{"accuracy", "tool_trajectory"},
			}, reqs[0].body, "the spec must serialize as agent_id/dataset_id/metrics, with metrics as a JSON array")
		})
	}
}

func TestEvaluationRunList_AgentFilterQueryParam(t *testing.T) {
	t.Run("agent_id is sent when non-empty", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, _ *http.Request) {
			m.writeJSON(w, http.StatusOK, `{"data":[{"id":"run-1","agent_id":"agent-1","status":"succeeded"}],"links":{"next":null}}`)
		}
		c, _ := m.client(t)

		page, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "agent-1", 0)

		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, "run-1", page.Items[0].ID)
		assert.Equal(t, "agent-1", page.Items[0].AgentID)
		assert.Equal(t, "succeeded", page.Items[0].Status)
		assert.NotEmpty(t, page.Items[0].Record)

		reqs := m.recorded()
		require.Len(t, reqs, 1)
		assert.Equal(t, "agent-1", reqs[0].query.Get("agent_id"))
		assert.Equal(t, strconv.Itoa(transport.ListPageSize), reqs[0].query.Get("page_limit"))
	})

	t.Run("no filter means no agent_id parameter", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, _ *http.Request) {
			m.writeJSON(w, http.StatusOK, `{"data":[],"links":{"next":null}}`)
		}
		c, _ := m.client(t)

		_, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "", 0)

		require.NoError(t, err)
		reqs := m.recorded()
		require.Len(t, reqs, 1)
		assert.False(t, reqs[0].query.Has("agent_id"), "an empty agent filter must not be sent")
	})

	t.Run("a hostile agent filter is rejected before any request", func(t *testing.T) {
		c, _ := newEvalRunMock(t).client(t)

		_, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "../x", 0)

		var ce *clierr.CLIError
		require.True(t, errors.As(err, &ce))
		assert.Contains(t, err.Error(), "invalid agent id")
	})
}

func TestEvaluationRunList_Paginates(t *testing.T) {
	// pageOne serves one run and a links.next cursor pointing at the mock's
	// own second page; pageTwo serves the tail.
	pageOne := func(m *evalRunMock, agent string) string {
		next := m.srv.URL + evalRunCollectionPath + "?page_token=tok-2"
		return fmt.Sprintf(`{"data":[{"id":"run-1","agent_id":%q,"status":"succeeded"}],"links":{"next":%q}}`, agent, next)
	}
	pageTwo := func(agent string) string {
		return fmt.Sprintf(`{"data":[{"id":"run-2","agent_id":%q,"status":"failed"}],"links":{"next":null}}`, agent)
	}

	t.Run("unfiltered walk follows the cursor", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page_token") == "" {
				m.writeJSON(w, http.StatusOK, pageOne(m, "agent-1"))
				return
			}
			m.writeJSON(w, http.StatusOK, pageTwo("agent-1"))
		}
		c, _ := m.client(t)

		page, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "", 0)

		require.NoError(t, err)
		require.Len(t, page.Items, 2)
		assert.Equal(t, []string{"run-1", "run-2"}, []string{page.Items[0].ID, page.Items[1].ID})
		assert.False(t, page.LimitReached)
		assert.False(t, page.PageCapReached)

		reqs := m.recorded()
		require.Len(t, reqs, 2)
		assert.Empty(t, reqs[0].query.Get("page_token"))
		assert.Equal(t, "tok-2", reqs[1].query.Get("page_token"))
	})

	t.Run("the agent filter rides on every page", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page_token") == "" {
				m.writeJSON(w, http.StatusOK, pageOne(m, "agent-1"))
				return
			}
			m.writeJSON(w, http.StatusOK, pageTwo("agent-1"))
		}
		c, _ := m.client(t)

		page, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "agent-1", 0)

		require.NoError(t, err)
		require.Len(t, page.Items, 2)

		reqs := m.recorded()
		require.Len(t, reqs, 2)
		for i, req := range reqs {
			assert.Equal(t, "agent-1", req.query.Get("agent_id"), "page %d must carry the filter", i+1)
		}
		assert.Equal(t, "tok-2", reqs[1].query.Get("page_token"))
	})

	t.Run("limit stops the walk and reports it", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, _ *http.Request) {
			m.writeJSON(w, http.StatusOK, pageOne(m, "agent-1"))
		}
		c, _ := m.client(t)

		page, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "", 1)

		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.True(t, page.LimitReached, "one item was returned while the API had more")
		assert.False(t, page.PageCapReached)
		assert.Len(t, m.recorded(), 1, "the walk stops once the limit is reached")
	})

	t.Run("the agent filter rides on a limit-truncated walk", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, _ *http.Request) {
			m.writeJSON(w, http.StatusOK, pageOne(m, "agent-1"))
		}
		c, _ := m.client(t)

		page, err := c.EvaluationRuns().List(context.Background(), evalRunScope, "agent-1", 1)

		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.True(t, page.LimitReached)
		reqs := m.recorded()
		require.Len(t, reqs, 1, "the walk stops once the limit is reached")
		assert.Equal(t, "agent-1", reqs[0].query.Get("agent_id"), "the filter must ride the truncated walk's page")
	})

	t.Run("a never-ending cursor reports the page cap", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			agentID string
		}{
			{name: "unfiltered", agentID: ""},
			{name: "agent-filtered", agentID: "agent-1"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m := newEvalRunMock(t)
				var pages atomic.Int32
				m.respond = func(w http.ResponseWriter, _ *http.Request) {
					n := pages.Add(1)
					next := fmt.Sprintf("%s%s?page_token=tok-%d", m.srv.URL, evalRunCollectionPath, n)
					m.writeJSON(w, http.StatusOK, fmt.Sprintf(`{"data":[{"id":"run-%d","status":"running"}],"links":{"next":%q}}`, n, next))
				}
				c, _ := m.client(t)

				page, err := c.EvaluationRuns().List(context.Background(), evalRunScope, tc.agentID, 0)

				require.NoError(t, err)
				assert.True(t, page.PageCapReached, "an endless cursor must stop at the page cap")
				assert.Len(t, page.Items, MaxListPages)
				reqs := m.recorded()
				require.Len(t, reqs, MaxListPages)
				for i, req := range reqs {
					assert.Equal(t, tc.agentID, req.query.Get("agent_id"), "page %d", i+1)
				}
			})
		}
	})
}

func TestEvaluationRunGet_IncludesQuestionResults(t *testing.T) {
	m := newEvalRunMock(t)
	m.respond = func(w http.ResponseWriter, _ *http.Request) {
		m.writeJSON(w, http.StatusOK, `{"data":{"id":"run-1","agent_id":"agent-1","dataset_id":"ds-1","status":"succeeded","questions":[{"query_number":1,"query_text":"q","expected_answer":"a","metrics":[{"name":"accuracy","score":1.0}]}]}}`)
	}
	c, _ := m.client(t)

	run, err := c.EvaluationRuns().Get(context.Background(), evalRunScope, "run-1")

	require.NoError(t, err)
	assert.Equal(t, "run-1", run.ID)
	assert.Equal(t, "succeeded", run.Status)

	questions, ok := run.Record["questions"].([]any)
	require.True(t, ok, "Get's Record carries the per-question results")
	require.Len(t, questions, 1)
	first, ok := questions[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), first["query_number"])

	reqs := m.recorded()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodGet, reqs[0].method)
	assert.Equal(t, evalRunCollectionPath+"/run-1", reqs[0].path)
}

func TestEvaluationRunCancel(t *testing.T) {
	m := newEvalRunMock(t)
	m.respond = func(w http.ResponseWriter, _ *http.Request) {
		m.writeJSON(w, http.StatusOK, `{"data":{"id":"run-1","status":"cancelled"}}`)
	}
	c, _ := m.client(t)

	run, err := c.EvaluationRuns().Cancel(context.Background(), evalRunScope, "run-1")

	require.NoError(t, err)
	assert.Equal(t, "run-1", run.ID)
	assert.Equal(t, "cancelled", run.Status)

	reqs := m.recorded()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodPost, reqs[0].method)
	assert.Equal(t, evalRunCollectionPath+"/run-1/cancel", reqs[0].path)
}

func TestEvaluationRunDelete(t *testing.T) {
	m := newEvalRunMock(t)
	m.respond = func(w http.ResponseWriter, _ *http.Request) {
		m.writeJSON(w, http.StatusOK, `{}`)
	}
	c, _ := m.client(t)

	require.NoError(t, c.EvaluationRuns().Delete(context.Background(), evalRunScope, "run-1"))

	reqs := m.recorded()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodDelete, reqs[0].method)
	assert.Equal(t, evalRunCollectionPath+"/run-1", reqs[0].path)
}

func TestEvaluationRunWaitWhileRunning(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []string
		wantStatus string
		wantPolls  int
	}{
		{name: "queued to succeeded", statuses: []string{"queued", "running", "succeeded"}, wantStatus: "succeeded", wantPolls: 3},
		{name: "queued to failed", statuses: []string{"queued", "failed"}, wantStatus: "failed", wantPolls: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newEvalRunMock(t)
			var polls atomic.Int32
			m.respond = func(w http.ResponseWriter, _ *http.Request) {
				i := int(polls.Add(1)) - 1
				status := tc.statuses[min(i, len(tc.statuses)-1)]
				m.writeJSON(w, http.StatusOK, fmt.Sprintf(`{"data":{"id":"run-1","status":%q}}`, status))
			}
			c, cfg := m.client(t)
			cfg.AuraRuntime.SetPollingConfig(10, 0)

			status, err := c.EvaluationRuns().WaitWhileRunning(context.Background(), evalRunScope, "run-1")

			require.NoError(t, err)
			assert.Equal(t, tc.wantStatus, status)
			reqs := m.recorded()
			assert.Len(t, reqs, tc.wantPolls, "polling stops on the first terminal status")
			for _, req := range reqs {
				assert.Equal(t, http.MethodGet, req.method)
				assert.Equal(t, evalRunCollectionPath+"/run-1", req.path)
			}
		})
	}

	t.Run("a cancelled context stops the wait", func(t *testing.T) {
		m := newEvalRunMock(t)
		m.respond = func(w http.ResponseWriter, _ *http.Request) {
			m.writeJSON(w, http.StatusOK, `{"data":{"id":"run-1","status":"running"}}`)
		}
		c, cfg := m.client(t)
		cfg.AuraRuntime.SetPollingConfig(5, 3600) // an hour between attempts

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := c.EvaluationRuns().WaitWhileRunning(ctx, evalRunScope, "run-1")

		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, m.recorded(), "a cancelled context must not start a poll")
	})
}

func TestEvaluationRunValidateIDsBeforeAnyRequest(t *testing.T) {
	// A zero-value service has a nil config: any transport call would panic,
	// so a returned validation error proves no request was attempted.
	s := evaluationRunService{}
	good := Scope{OrgID: "org-1", ProjectID: "proj-1"}
	ctx := context.Background()

	cases := map[string]func() error{
		"list bad agent filter": func() error { _, err := s.List(ctx, good, "a/b", 0); return err },
		"list bad org":          func() error { _, err := s.List(ctx, Scope{OrgID: "..", ProjectID: "p"}, "", 0); return err },
		"get bad run id":        func() error { _, err := s.Get(ctx, good, "../x"); return err },
		"create bad project": func() error {
			_, err := s.Create(ctx, Scope{OrgID: "o", ProjectID: "a/b"}, EvaluationRunSpec{})
			return err
		},
		"delete bad run id": func() error { return s.Delete(ctx, good, "a?b") },
		"cancel bad run id": func() error { _, err := s.Cancel(ctx, good, "a/b"); return err },
		"wait bad run id":   func() error { _, err := s.WaitWhileRunning(ctx, good, "a/b"); return err },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			err := call()
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "want a clierr, got %v", err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}

func TestEvaluationRunList_RejectsNegativeLimit(t *testing.T) {
	// A zero-value service has a nil config: a returned usage error proves no
	// request was attempted.
	_, err := evaluationRunService{}.List(context.Background(), evalRunScope, "", -1)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, err.Error(), "--limit must be zero or greater")
}
