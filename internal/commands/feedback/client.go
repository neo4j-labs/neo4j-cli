// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Outcomes of a submit attempt.
const (
	outcomeSent     = "sent"
	outcomeSkipped  = "skipped"
	outcomeRejected = "rejected"
	outcomeFailed   = "failed"
	outcomeUnknown  = "unknown"
)

// Reasons.
const (
	reasonInvalidInput    = "invalid_input"
	reasonPolicyViolation = "policy_violation"
	reasonEndpointDenied  = "endpoint_not_allowed"
	reasonServerRejected  = "server_rejected"
	reasonServerError     = "server_error"
	reasonUnreachable     = "unreachable"
	reasonTimeout         = "timeout"
	reasonNoReceipt       = "no_receipt"
	reasonNotConfigured   = "not_configured"
	reasonCancelled       = "cancelled"
)

// eventName is the Mixpanel event a report is sent as. It follows the CLI's
// existing NEO4J-CLI_<NAME> convention (NEO4J-CLI_COMMAND, NEO4J-CLI_QUERY) and must match Lexicon exactly.
const eventName = "NEO4J-CLI_AGENT_FEEDBACK"

// Set at build time with -ldflags "-X .../feedback.mixpanelToken=<project token>"
// (and mixpanelBase for another region). A build without a token has no
// destination of its own and can only send to a loopback test endpoint.
var (
	mixpanelToken = ""
	mixpanelBase  = "https://api.mixpanel.com"
)

// sendTimeout is the total deadline for one submission. A var so tests can
// shorten it.
var sendTimeout = 3 * time.Second

const maxResponseBytes = 64 * 1024

// Result is the classified outcome of one POST.
type Result struct {
	Outcome   string
	Reason    string
	ReportID  string
	ElapsedMS int64
}

// proxyFunc picks the proxy for a request: HTTPS_PROXY for https endpoints
// (HTTP_PROXY for http) and NO_PROXY, as in the standard library. Loopback
// destinations are never proxied. A var so tests can substitute one.
var proxyFunc = http.ProxyFromEnvironment

// httpClient honors the proxy environment variables but never follows
// redirects (a redirect, from the endpoint or from a proxy, could send the
// report somewhere else).
var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy:              func(r *http.Request) (*url.URL, error) { return proxyFunc(r) },
		DisableCompression: true,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// trackEvent builds the one-element Mixpanel /track body for a report. The
// random request id is both distinct_id (one report is one "user", so one
// report can be deleted on its own) and $insert_id (deduplication). No `time`
// and no IP are sent (the request carries ip=0), and nothing identifies the
// machine or the person.
func trackEvent(token string, p Payload) []map[string]any {
	props := map[string]any{
		"token":          token,
		"distinct_id":    p.RequestID,
		"$insert_id":     p.RequestID,
		"schema_version": p.SchemaVersion,
		"surface":        p.Surface,
		"cli_version":    p.CLIVersion,
		"category":       p.Category,
		"expected":       p.Expected,
		"observed":       p.Observed,
	}
	if p.OS != "" {
		props["$os"] = p.OS // the same property name the CLI's analytics events use
	}
	if p.OSArch != "" {
		props["os_arch"] = p.OSArch
	}
	if p.Invoker != "" {
		props["invoker"] = p.Invoker
	}
	if p.CommandID != "" {
		props["command_id"] = p.CommandID
	}
	if p.ErrorCode != "" {
		props["error_code"] = p.ErrorCode
	}
	if p.Agent != nil {
		if p.Agent.Harness != "" {
			props["agent_harness"] = p.Agent.Harness
		}
		if p.Agent.Model != "" {
			props["agent_model"] = p.Agent.Model
		}
	}
	return []map[string]any{{"event": eventName, "properties": props}}
}

// classifyTrack maps a /track response to an outcome. With verbose=1 Mixpanel
// answers {"status":1,"error":null} on success and {"status":0,"error":"..."}
// when no event in the body was valid; without it, the bare integer 1 or 0.
// The error text is never surfaced.
func classifyTrack(data []byte) (outcome, reason string) {
	trimmed := bytes.TrimSpace(data)
	switch string(trimmed) {
	case "1":
		return outcomeSent, ""
	case "0":
		return outcomeRejected, reasonServerRejected
	}
	var parsed struct {
		Status *int `json:"status"`
	}
	if json.Unmarshal(trimmed, &parsed) != nil || parsed.Status == nil {
		return outcomeUnknown, reasonNoReceipt
	}
	if *parsed.Status == 1 {
		return outcomeSent, ""
	}
	return outcomeRejected, reasonServerRejected
}

// Send makes exactly one POST of payload to endpoint's /track under a 3 s
// total deadline and classifies the response. It never retries.
func Send(ctx context.Context, endpoint string, payload Payload) Result {
	// Send checks the destination itself too: it is exported, and a report
	// must never reach anything endpointAllowed refuses.
	target, ok := trackURL(endpoint)
	if !ok {
		return Result{Outcome: outcomeFailed, Reason: reasonEndpointDenied}
	}
	token := mixpanelToken
	if token == "" && !isLoopbackEndpoint(endpoint) {
		return Result{Outcome: outcomeFailed, Reason: reasonNotConfigured}
	}
	body, err := json.Marshal(trackEvent(token, payload))
	if err != nil {
		return Result{Outcome: outcomeFailed, Reason: reasonUnreachable}
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Result{Outcome: outcomeFailed, Reason: reasonUnreachable}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("User-Agent", "neo4j-cli/"+payload.CLIVersion+" feedback")

	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		res := Result{ElapsedMS: time.Since(start).Milliseconds()}
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			res.Outcome, res.Reason = outcomeUnknown, reasonTimeout
		case errors.Is(err, context.Canceled):
			// Cancelled mid-request (the person pressed Esc): the server may
			// already have the report, so this is not a failure to retry.
			res.Outcome, res.Reason = outcomeUnknown, reasonCancelled
		default:
			res.Outcome, res.Reason = outcomeFailed, reasonUnreachable
		}
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	res := Result{ElapsedMS: time.Since(start).Milliseconds()}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if readErr != nil && errors.Is(readErr, context.DeadlineExceeded) {
			res.Outcome, res.Reason = outcomeUnknown, reasonTimeout
			return res
		}
		res.Outcome, res.Reason = classifyTrack(data)
		if res.Outcome == outcomeSent {
			res.ReportID = payload.RequestID
		}
	case resp.StatusCode == http.StatusTooManyRequests:
		// Rate limited: nothing was recorded and a later attempt may work.
		res.Outcome, res.Reason = outcomeFailed, reasonServerError
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		res.Outcome, res.Reason = outcomeRejected, reasonServerRejected
	default:
		// 5xx, and anything unexpected (1xx, 3xx since redirects are not followed).
		res.Outcome, res.Reason = outcomeFailed, reasonServerError
	}
	return res
}
