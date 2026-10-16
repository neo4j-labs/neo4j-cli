// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neo4j/cli/internal/clicfg"
)

// First-report consent. When feedback_submit is called while feedback is not
// authorized, the server asks the person through the client's own UI (MCP
// elicitation) instead of skipping. A client that implements elicitation shows
// the question to the person, not to the agent, so the agent cannot answer it. Only the MCP tool path asks: `feedback submit`
// on the shell never does.
const (
	keyDeclinedUntil = "feedback.declined-until"
	declineCooldown  = 7 * 24 * time.Hour

	reasonUserDeclined     = "user_declined"
	reasonConsentCancelled = "consent_cancelled"
	reasonConsentNoUI      = "consent_unavailable"
	reasonConsentTimeout   = "consent_timeout"
	reasonDismissed        = "consent_dismissed"

	hintEnable       = "Feedback is not enabled and could not ask the user here. A person can enable it with: neo4j-cli feedback enable"
	hintDoNotTrack   = "DO_NOT_TRACK=1 is set, so feedback is off. Do not try to work around it; continue the task."
	hintTelemetryOff = "Telemetry is turned off in neo4j-cli's settings, so feedback is off. Do not try to work around it; continue the task."
	hintDeclined     = "The user declined. Do not ask again or retry; continue the task."
	hintNoAnswer     = "The user did not answer the question, so nothing was sent. Do not retry on your own; continue the task. If the user later asks you to send it, call the tool again."
	hintDismissed    = "The user dismissed the question, so nothing was sent, and it will not be asked again in this session. Do not retry; continue the task. If the user wants to send it, they can start a new agent session, or run neo4j-cli feedback enable in a separate terminal."
)

// consentMessage is laid out for how Claude Code shows it: only the first few
// lines are visible and the rest is collapsed behind "(+N more lines)". So the
// first lines must carry what the person is agreeing to: the consequence (this
// report, plus more like it until the expiry), the destination, and the report
// text itself. Details come after.
const consentMessage = "Send this report to Neo4j, and allow coding agents on this machine to send reports until <date>? Destination: <endpoint>\n" +
	"expected: <expected>\n" +
	"observed: <observed>\n" +
	"category: <category>. Decline sends nothing and you won't be asked again for 7 days. Revoke any time: neo4j-cli feedback disable\n" +
	"\n" +
	"A coding agent wrote this report about neo4j-cli. Accepting covers reports from any coding agent on this machine and from neo4j-cli feedback submit until the date above. " +
	"Reports contain only: category, expected, observed, optional command name, error code and agent model, " +
	"the agent harness, the CLI version, the operating system and CPU architecture, how the CLI was run (agent, script or person) and a random request id. They are checked locally and rejected if they appear to contain query text, " +
	"parameters, schema names, output, file paths, URIs, hostnames, credentials, emails, names, or identifiers.\n" +
	"Every report sent is recorded in full at: <receipts>"

// consentResult is the answer to a consent request.
type consentResult struct {
	Granted bool
	Reason  string // set when not granted
	Hint    string
}

// consentFunc asks a person whether to allow sending p to ep until expiry.
type consentFunc func(ctx context.Context, p Payload, ep string, expiry time.Time) consentResult

func renderConsent(p Payload, ep string, expiry time.Time) string {
	return strings.NewReplacer(
		"<endpoint>", ep,
		"<category>", p.Category,
		"<expected>", p.Expected,
		"<observed>", p.Observed,
		"<date>", expiry.Format("2006-01-02 15:04 MST"),
		"<receipts>", receiptsPath(),
	).Replace(consentMessage)
}

// consentTimeout is how long the server waits for the person to answer. After
// it, nothing is sent and no decline is recorded: silence is not a decision.
var consentTimeout = 5 * time.Minute

// consentSession remembers, for one `feedback serve` process (one agent
// session), that the person dismissed the question, so the server does not ask
// again until the next session. A question left unanswered until the time limit
// is not remembered: the person was away, not saying no, so the next report may
// ask again.
type consentSession struct {
	dismissed atomic.Bool
}

// elicitConsent returns a consentFunc that asks through ss. It grants only on an
// explicit "accept" that arrives within consentTimeout. A client that does not
// advertise elicitation, or a failed request, is treated as "no consent
// possible", never as consent.
func elicitConsent(ss *mcpsdk.ServerSession, cs *consentSession) consentFunc {
	return func(ctx context.Context, p Payload, ep string, expiry time.Time) consentResult {
		if ss == nil {
			return consentResult{Reason: reasonConsentNoUI, Hint: hintEnable}
		}
		if cs.dismissed.Load() {
			return consentResult{Reason: reasonDismissed, Hint: hintDismissed}
		}
		ip := ss.InitializeParams()
		if ip == nil || ip.Capabilities == nil || ip.Capabilities.Elicitation == nil {
			return consentResult{Reason: reasonConsentNoUI, Hint: hintEnable}
		}
		askCtx, cancel := context.WithTimeout(ctx, consentTimeout)
		defer cancel()
		res, err := ss.Elicit(askCtx, &mcpsdk.ElicitParams{
			Mode:            "form",
			Message:         renderConsent(p, ep, expiry),
			RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		})
		if ctx.Err() == nil && errors.Is(askCtx.Err(), context.DeadlineExceeded) {
			// An answer that arrives after this is dropped.
			return consentResult{Reason: reasonConsentTimeout, Hint: hintNoAnswer}
		}
		if err != nil || res == nil {
			return consentResult{Reason: reasonConsentNoUI, Hint: hintEnable}
		}
		switch res.Action {
		case "accept":
			return consentResult{Granted: true}
		case "decline":
			return consentResult{Reason: reasonUserDeclined, Hint: hintDeclined}
		default:
			cs.dismissed.Store(true)
			return consentResult{Reason: reasonConsentCancelled, Hint: hintDismissed}
		}
	}
}

// declinedRecently reports whether the person declined within the cooldown.
func declinedRecently(cfg *clicfg.Config, at time.Time) bool {
	t, err := time.Parse(time.RFC3339, getString(cfg, keyDeclinedUntil))
	return err == nil && at.Before(t)
}

func recordDecline(cfg *clicfg.Config, at time.Time) error {
	return setValues(cfg, [2]string{keyDeclinedUntil, at.Add(declineCooldown).UTC().Format(time.RFC3339)})
}

// authorize stores a consented authorization, as `feedback enable --accept` would.
func authorize(cfg *clicfg.Config, ep string, expiry time.Time) error {
	if err := setValues(cfg,
		[2]string{keyEndpoint, ep},
		[2]string{keyAuthorizedUntil, expiry.UTC().Format(time.RFC3339)},
		[2]string{keyDeclinedUntil, ""},
	); err != nil {
		return fmt.Errorf("storing feedback authorization: %w", err)
	}
	return nil
}
