// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
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

// consentRig is a tool server with feedback NOT enabled, a counting endpoint,
// and a client whose elicitation answers are scripted.
type consentRig struct {
	cs      *mcpsdk.ClientSession
	cfg     *clicfg.Config
	hits    *atomic.Int32
	asked   *atomic.Int32
	message *atomic.Value
}

func newConsentRig(t *testing.T, withElicitation bool, action string) *consentRig {
	t.Helper()
	withSeams(t, false, "claude-code")
	r := &consentRig{hits: &atomic.Int32{}, asked: &atomic.Int32{}, message: &atomic.Value{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		r.hits.Add(1)
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"status":1,"error":null}`))
	}))
	t.Cleanup(srv.Close)
	prev := lookupEnv
	t.Cleanup(func() { lookupEnv = prev })
	lookupEnv = func(k string) string {
		if k == endpointEnv {
			return srv.URL
		}
		return ""
	}
	r.cfg = newTestConfig(t)
	fb := newFeedbackServer(r.cfg, newTestApp(r.cfg))
	st, ct := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ss, err := fb.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	var opts *mcpsdk.ClientOptions
	if withElicitation {
		opts = &mcpsdk.ClientOptions{ElicitationHandler: func(_ context.Context, req *mcpsdk.ElicitRequest) (*mcpsdk.ElicitResult, error) {
			r.asked.Add(1)
			r.message.Store(req.Params.Message)
			if action == "late-accept" {
				// The person answers only after the server stopped waiting.
				time.Sleep(200 * time.Millisecond)
				return &mcpsdk.ElicitResult{Action: "accept"}, nil
			}
			return &mcpsdk.ElicitResult{Action: action}, nil
		}}
	}
	r.cs, err = mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, opts).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.cs.Close() })
	return r
}

func (r *consentRig) report(t *testing.T, expected string) map[string]any {
	t.Helper()
	res, err := r.cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: feedbackToolName,
		Arguments: map[string]any{
			"category": "missing_capability", "expected": expected,
			"observed": "Only default, json, table and toon formats exist.", "command": "query",
		},
	})
	require.NoError(t, err)
	return toolOutcome(t, res)
}

const csvExpected = "A CSV output format for query results."

func TestConsent_AcceptSendsAndAuthorizes(t *testing.T) {
	r := newConsentRig(t, true, "accept")
	out := r.report(t, csvExpected)
	assert.Equal(t, "sent", out["outcome"])
	assert.EqualValues(t, 1, r.hits.Load())
	assert.True(t, Authorized(r.cfg, time.Now()), "accepting stores a 30-day authorization")

	// The question carries the exact report, the destination and the revoke command.
	msg, _ := r.message.Load().(string)
	for _, want := range []string{csvExpected, "Only default, json, table and toon formats exist.", "127.0.0.1", "neo4j-cli feedback disable"} {
		assert.Contains(t, msg, want)
	}

	// Authorized now: the next report is sent without asking again.
	out = r.report(t, "A second distinct problem report text.")
	assert.Equal(t, "sent", out["outcome"])
	assert.EqualValues(t, 1, r.asked.Load())
	assert.EqualValues(t, 2, r.hits.Load())
}

func TestConsent_DeclineSendsNothingAndIsRemembered(t *testing.T) {
	r := newConsentRig(t, true, "decline")
	out := r.report(t, csvExpected)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "user_declined", out["reason"])
	assert.Contains(t, out["hint"], "Do not ask again")
	assert.EqualValues(t, 0, r.hits.Load())
	assert.False(t, Authorized(r.cfg, time.Now()))
	assert.True(t, declinedRecently(r.cfg, time.Now()))

	// Within the cooldown the person is not asked again.
	out = r.report(t, "Another problem text to report here.")
	assert.Equal(t, "user_declined", out["reason"])
	assert.EqualValues(t, 1, r.asked.Load())
	assert.EqualValues(t, 0, r.hits.Load())
}

func TestConsent_DeclineExpiresAfterCooldown(t *testing.T) {
	r := newConsentRig(t, true, "decline")
	r.report(t, csvExpected)
	prev := now
	t.Cleanup(func() { now = prev })
	now = func() time.Time { return time.Now().Add(declineCooldown + time.Hour) }
	r.report(t, "Asked again once the cooldown is over.")
	assert.EqualValues(t, 2, r.asked.Load())
}

func TestConsent_CancelSendsNothingAndIsNotAskedAgainThisSession(t *testing.T) {
	r := newConsentRig(t, true, "cancel")
	out := r.report(t, csvExpected)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "consent_cancelled", out["reason"])
	assert.Contains(t, out["hint"], "will not be asked again in this session")
	assert.Contains(t, out["hint"], "neo4j-cli feedback enable", "the hint says how the person can still send")
	assert.EqualValues(t, 0, r.hits.Load())
	assert.False(t, declinedRecently(r.cfg, time.Now()), "a dismissal is not a decline")

	out = r.report(t, "Second attempt after a dismissed question.")
	assert.Equal(t, "consent_dismissed", out["reason"])
	assert.EqualValues(t, 1, r.asked.Load(), "the same session does not ask again")

	// A new session (a new server) asks again.
	r2 := newConsentRig(t, true, "cancel")
	r2.report(t, csvExpected)
	assert.EqualValues(t, 1, r2.asked.Load())
}

func TestConsent_NoAnswerTimesOutSendsNothingAndAsksAgainNextTime(t *testing.T) {
	prev := consentTimeout
	t.Cleanup(func() { consentTimeout = prev })
	consentTimeout = 50 * time.Millisecond

	r := newConsentRig(t, true, "late-accept")
	out := r.report(t, csvExpected)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "consent_timeout", out["reason"])
	assert.Contains(t, out["hint"], "did not answer")
	assert.False(t, declinedRecently(r.cfg, time.Now()), "silence is not a decline")

	time.Sleep(300 * time.Millisecond) // the late "accept" arrives and is dropped
	assert.EqualValues(t, 0, r.hits.Load(), "nothing is sent")
	assert.False(t, Authorized(r.cfg, time.Now()), "a late accept authorizes nothing")

	// The person was away, not saying no: a later report asks again.
	out = r.report(t, "Second attempt after an unanswered question.")
	assert.Equal(t, "consent_timeout", out["reason"])
	assert.EqualValues(t, 2, r.asked.Load(), "a timeout does not stop the next question")
}

func TestConsent_ClientWithoutElicitationNeverEnables(t *testing.T) {
	r := newConsentRig(t, false, "")
	out := r.report(t, csvExpected)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "consent_unavailable", out["reason"])
	assert.Contains(t, out["hint"], "neo4j-cli feedback enable")
	assert.EqualValues(t, 0, r.hits.Load())
	assert.False(t, Authorized(r.cfg, time.Now()))
}

func TestConsent_RejectedReportIsNotAskedAbout(t *testing.T) {
	r := newConsentRig(t, true, "accept")
	out := r.report(t, "Run MATCH (n) RETURN n against bolt://db.example.com")
	assert.Equal(t, "rejected", out["outcome"])
	assert.EqualValues(t, 0, r.asked.Load(), "nobody is asked to approve a report that would be rejected")
	assert.EqualValues(t, 0, r.hits.Load())
	assert.False(t, Authorized(r.cfg, time.Now()))
}

func TestConsent_NoEndpointKnownSkipsWithoutAsking(t *testing.T) {
	r := newConsentRig(t, true, "accept")
	lookupEnv = func(string) string { return "" }
	out := r.report(t, csvExpected)
	assert.Equal(t, "skipped", out["outcome"])
	assert.EqualValues(t, 0, r.asked.Load())
}

func TestConsent_ShellSubmitNeverAsks(t *testing.T) {
	withSeams(t, false, "")
	cfg := newTestConfig(t)
	stdout, _, _ := runCmd(t, newTestApp(cfg), "feedback", "submit", "--category", "other_friction",
		"--expected", csvExpected, "--observed", "Only default, json, table and toon formats exist.")
	out := parseJSON(t, stdout)
	assert.Equal(t, "skipped", out["outcome"])
	assert.Equal(t, "not_enabled", out["reason"])
	assert.False(t, strings.Contains(stdout, "consent"))
}

func TestRenderConsent_KeyContentIsInTheFirstFourLines(t *testing.T) {
	// Claude Code collapses everything after the first few lines.
	p := Payload{Category: "missing_capability", Expected: csvExpected, Observed: "Only default, json, table and toon formats exist."}
	msg := renderConsent(p, "https://feedback.example/v1", time.Date(2026, 11, 2, 0, 59, 0, 0, time.UTC))
	top := strings.Join(strings.SplitN(msg, "\n", 5)[:4], "\n")
	for _, want := range []string{"2026-11-02", "https://feedback.example/v1", csvExpected, "Only default, json, table and toon formats exist.", "Decline sends nothing", "neo4j-cli feedback disable"} {
		assert.Contains(t, top, want)
	}
	assert.NotContains(t, msg, "<", "every placeholder is filled in")
	assert.Contains(t, msg, receiptsPath(), "the full record location is in the message")
}
