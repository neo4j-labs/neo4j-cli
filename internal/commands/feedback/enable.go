// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
)

const (
	defaultDays = 30
	maxDays     = 90
)

func newEnableCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		accept   bool
		ep       string
		days     int
		register bool
		instr    string
	)

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Authorize sending feedback reports (operator only)",
		Long: "Authorize this machine to send feedback reports for a limited time. " +
			"This is an operator action and is refused when an agent harness is detected. " +
			"Without --accept it prints what would be sent and exits with a usage error. " +
			"Only the destination built into this build, or a loopback test endpoint, is accepted.",
		Example: `# Show what enabling would authorize (exits 2 until --accept is given)
neo4j-cli feedback enable --rw

# Enable for 30 days and print the resulting state as JSON
neo4j-cli feedback enable --accept --rw --format json`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			// The privacy settings are checked first, so that an installer
			// can tell "switched off" (exit 2, nothing on stdout) from "an
			// agent is running this" (exit 4).
			switch optOut(cfg) {
			case reasonDoNotTrack:
				return clierr.NewUsageError("feedback not enabled: DO_NOT_TRACK=1 is set; unset it to enable feedback")
			case reasonTelemetryOff:
				return clierr.NewUsageError("feedback not enabled: telemetry is turned off in neo4j-cli's settings; feedback follows that setting")
			}
			if detectAgent() {
				return clierr.NewAuthError("feedback can only be enabled by the operator, outside an agent session")
			}
			if ep == "" {
				ep = fallbackEndpoint()
			}
			if ep == "" {
				return clierr.NewUsageError("this build has no feedback destination; pass --endpoint (http on 127.0.0.1, localhost or ::1 only)")
			}
			if !endpointAllowed(ep) {
				return clierr.NewUsageError("endpoint not allowed: only https to Mixpanel's ingestion hosts, or http on 127.0.0.1, localhost or ::1, is accepted")
			}
			if days < 1 || days > maxDays {
				return clierr.NewUsageError("--days must be between 1 and %d", maxDays)
			}
			if !validInstructionsVariant(instr) {
				return clierr.NewUsageError("--instructions must be none, basic or optin")
			}

			expiry := now().UTC().Add(time.Duration(days) * 24 * time.Hour)
			if accept && invokerLabel() != "human" {
				// Agent detection only sees known environment variables; also
				// requiring a terminal on stdin keeps --accept a person's act.
				return clierr.NewAuthError("feedback can only be enabled by a person at a terminal: run neo4j-cli feedback enable --accept yourself")
			}
			if !accept {
				text := strings.NewReplacer("<endpoint>", ep, "<date>", expiry.Format("2006-01-02 15:04 MST")).Replace(enableDisclosure)
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), text)
				return clierr.NewUsageError("feedback not enabled: re-run with --accept to enable")
			}

			if err := setValues(cfg,
				[2]string{keyEndpoint, ep},
				[2]string{keyInstructions, instr},
				[2]string{keyAutoRegistered, autoDone},
				[2]string{keyAuthorizedUntil, expiry.Format(time.RFC3339)},
			); err != nil {
				return clierr.NewFatalError("could not enable feedback, the config file is not writable: %s", err)
			}
			st := currentStatus(cfg)
			if register {
				st.RegisteredAgents, st.Notes = registerAgents(cmd.Context(), instr)
			}
			return writeStatus(cmd, cfg, st)
		},
	}

	cmd.Flags().StringVar(&instr, "instructions", enableInstructions, "Server instructions variant for the registered tool: none, basic or optin")
	_ = cmd.Flags().MarkHidden("instructions")
	cmd.Flags().BoolVar(&register, "register", true, "Register the feedback tool with detected agents (Claude Code, Codex); use --register=false to skip")
	cmd.Flags().BoolVar(&accept, "accept", false, "Accept the disclosure and enable feedback")
	cmd.Flags().StringVar(&ep, "endpoint", "", "Feedback endpoint URL (loopback test endpoint, or this build's destination)")
	cmd.Flags().IntVar(&days, "days", defaultDays, "Days until authorization expires (1-90)")
	return cmd
}
