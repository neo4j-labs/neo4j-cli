// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
)

// outcomeObj is the single JSON document `submit` prints on stdout.
type outcomeObj struct {
	Outcome       string      `json:"outcome"`
	Reason        string      `json:"reason"`
	RequestID     string      `json:"request_id"`
	ReportID      string      `json:"report_id"`
	ElapsedMS     int64       `json:"elapsed_ms"`
	Violations    []Violation `json:"violations"`
	DroppedFields []string    `json:"dropped_fields"`
	Payload       *Payload    `json:"payload"`
	ReceiptPath   string      `json:"receipt_path"`
	Hint          string      `json:"hint,omitempty"`
}

func newOutcome() outcomeObj {
	return outcomeObj{Violations: []Violation{}, DroppedFields: []string{}}
}

func newSubmitCmd(cfg *clicfg.Config) *cobra.Command {
	var in Input

	long := "Send one structured feedback report about neo4j-cli: a category, what you expected, and what you observed." +
		"\n\n" + dataPolicyBlock

	cmd := &cobra.Command{
		Use:   "submit",
		Short: submitShort,
		Long:  long,
		Example: `# Report a missing capability
neo4j-cli feedback submit --category missing_capability --expected "An output format suitable for spreadsheets" --observed "Only default, json, table and toon formats exist" --command query

# Report an unclear error and read the outcome as JSON
neo4j-cli feedback submit --category unclear_error --expected "The error should name the missing setting" --observed "The message did not say which setting to change" --error-code usage_error --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runSubmit(cmd, cfg, in)
		},
	}

	f := cmd.Flags()
	f.StringVar(&in.Category, "category", "", "Category: unclear_error, unexpected_behavior, missing_capability or other_friction")
	f.StringVar(&in.Expected, "expected", "", "What you expected neo4j-cli to do (product behavior, no query text or data)")
	f.StringVar(&in.Observed, "observed", "", "What neo4j-cli actually did (product behavior, no query text or data)")
	f.StringVar(&in.Command, "command", "", "Optional: the neo4j-cli command involved, e.g. query")
	f.StringVar(&in.ErrorCode, "error-code", "", "Optional: the CLI error code name, e.g. usage_error")
	f.StringVar(&in.AgentModel, "agent-model", "", "Optional: public model name, e.g. claude-opus-5-5")
	f.StringVar(&in.RequestID, "request-id", "", "Optional: reuse a previous request id (UUIDv4) to retry safely")
	return cmd
}

// runSubmit prints the outcome of submitReport and maps it to an exit code.
func runSubmit(cmd *cobra.Command, cfg *clicfg.Config, in Input) error {
	return finish(cmd, submitReport(cmd.Context(), cfg, cmd.Root(), in, nil))
}

// submitReport runs in this order: authorization, endpoint, validation,
// send, receipt. Nothing touches the network before validation passes. It is
// shared by `feedback submit` and the `feedback serve` tool. ask is nil on the
// shell path; the tool passes a consentFunc that asks the person when feedback
// is not yet authorized.
func submitReport(ctx context.Context, cfg *clicfg.Config, tree *cobra.Command, in Input, ask consentFunc) outcomeObj {
	fs := cfg.Fs()
	out := newOutcome()

	switch optOut(cfg) {
	case reasonDoNotTrack:
		out.Outcome, out.Reason, out.Hint = outcomeSkipped, reasonDoNotTrack, hintDoNotTrack
		return out
	case reasonTelemetryOff:
		out.Outcome, out.Reason, out.Hint = outcomeSkipped, reasonTelemetryOff, hintTelemetryOff
		return out
	}

	authReason := authState(cfg, now())
	if authReason != "" {
		if ask == nil {
			out.Outcome, out.Reason = outcomeSkipped, authReason
			return out
		}
		if declinedRecently(cfg, now()) {
			out.Outcome, out.Reason, out.Hint = outcomeSkipped, reasonUserDeclined, hintDeclined
			return out
		}
	}

	ep := endpoint(cfg)
	if authReason != "" && ep == "" {
		ep = fallbackEndpoint()
	}
	if authReason != "" && ep == "" {
		out.Outcome, out.Reason, out.Hint = outcomeSkipped, authReason, hintEnable
		return out
	}
	in.CLIVersion = cfg.Version
	in.Harness = harnessLabel()
	in.OS, in.OSArch, in.Invoker = goos, goarch, invokerLabel()

	if !endpointAllowed(ep) {
		out.RequestID, _ = resolveRequestID(in.RequestID)
		out.Outcome, out.Reason = outcomeRejected, reasonEndpointDenied
		out.ReceiptPath = appendReceipt(fs, receiptsPath(), Receipt{
			Outcome: out.Outcome, Reason: out.Reason, RequestID: out.RequestID, Destination: ep,
		})
		return out
	}

	payload, violations, dropped := Validate(in, tree)
	out.RequestID = payload.RequestID
	if dropped != nil {
		out.DroppedFields = dropped
	}
	if len(violations) > 0 {
		out.Violations = violations
		out.Outcome, out.Reason = outcomeRejected, rejectionReason(violations)
		out.ReceiptPath = appendReceipt(fs, receiptsPath(), Receipt{
			Outcome: out.Outcome, Reason: out.Reason, RequestID: out.RequestID, Destination: ep,
		})
		return out
	}

	if authReason != "" {
		expiry := now().UTC().Add(defaultDays * 24 * time.Hour)
		c := ask(ctx, payload, ep, expiry)
		if !c.Granted {
			if c.Reason == reasonUserDeclined {
				_ = recordDecline(cfg, now())
			}
			out.Outcome, out.Reason, out.Hint = outcomeSkipped, c.Reason, c.Hint
			out.ReceiptPath = appendReceipt(fs, receiptsPath(), Receipt{
				Outcome: out.Outcome, Reason: out.Reason, RequestID: out.RequestID, Destination: ep,
			})
			return out
		}
		if err := authorize(cfg, ep, expiry); err != nil {
			out.Outcome, out.Reason = outcomeFailed, "authorization_not_stored"
			return out
		}
	}

	res := Send(ctx, ep, payload)
	out.Outcome, out.Reason, out.ReportID, out.ElapsedMS = res.Outcome, res.Reason, res.ReportID, res.ElapsedMS
	if res.Outcome == outcomeSent {
		out.Payload = &payload
	}
	out.ReceiptPath = appendReceipt(fs, receiptsPath(), Receipt{
		Outcome: out.Outcome, Reason: out.Reason, RequestID: out.RequestID, ReportID: out.ReportID,
		Destination: ep, ElapsedMS: out.ElapsedMS, Payload: &payload,
	})
	return out
}

// rejectionReason is policy_violation when any detector fired, otherwise
// invalid_input (field rules and the size cap).
func rejectionReason(vs []Violation) string {
	for _, v := range vs {
		if v.Detector != detInvalidInput && v.Detector != detTooLarge && v.Detector != detTooLong {
			return reasonPolicyViolation
		}
	}
	return reasonInvalidInput
}

// finish prints the outcome document, then maps non-zero outcomes to a typed
// error (6 for rejected, 8 for failed and unknown).
func finish(cmd *cobra.Command, out outcomeObj) error {
	if err := writeJSON(cmd, out); err != nil {
		return err
	}
	switch out.Outcome {
	case outcomeSent, outcomeSkipped:
		return nil
	case outcomeRejected:
		return clierr.NewValidationError("feedback %s: %s", out.Outcome, out.Reason)
	default:
		return clierr.NewUpstreamError("feedback %s: %s", out.Outcome, out.Reason)
	}
}
