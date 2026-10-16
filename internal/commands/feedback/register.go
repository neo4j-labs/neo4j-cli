// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
)

// newRegisterCmd registers the feedback tool with the agent WITHOUT authorizing
// anything. Nothing can be sent until a person accepts the first-report prompt
// or runs `feedback enable`, so an agent may run this safely.
func newRegisterCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		ep    string
		instr string
	)
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register the feedback tool with coding agents (does not enable sending)",
		Long: "Register the feedback tool with detected coding agents (Claude Code, Codex) without authorizing anything. " +
			"Nothing is sent until a person accepts the question the tool asks the first time an agent tries to report, " +
			"or runs `neo4j-cli feedback enable`. It changes the agents' configuration, so like other writes it needs --rw under an agent.",
		Example: `# Register the tool; the first report will ask the person
neo4j-cli feedback register --rw

# Register and print the state as JSON
neo4j-cli feedback register --rw --format json`,
		Annotations: map[string]string{"write": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if cmd.Flags().Changed("instructions") && detectAgent() {
				return clierr.NewAuthError("the server instructions variant can only be chosen by a person, outside an agent session")
			}
			if Authorized(cfg, now()) {
				// An authorization covers the destination the person saw; keep
				// it, and refuse to point the authorization somewhere else.
				stored := endpoint(cfg)
				if cmd.Flags().Changed("endpoint") && ep != stored {
					return clierr.NewConflictError("feedback is enabled for %s; run neo4j-cli feedback disable first to change the destination", stored)
				}
				ep = stored
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
			if !validInstructionsVariant(instr) {
				return clierr.NewUsageError("--instructions must be none, basic or optin")
			}
			agents, notes, err := registerFor(cmd.Context(), cfg, ep, instr)
			if err != nil {
				return err
			}
			st := currentStatus(cfg)
			st.RegisteredAgents, st.Notes = agents, notes
			if !st.Enabled {
				st.Notes = append(st.Notes, "Not enabled yet: the first time an agent reports, the person is asked to accept or decline.")
			}
			return writeStatus(cmd, cfg, st)
		},
	}
	cmd.Flags().StringVar(&ep, "endpoint", "", "Feedback endpoint URL (loopback test endpoint, or this build's destination)")
	cmd.Flags().StringVar(&instr, "instructions", enableInstructions, "Server instructions variant: none, basic or optin")
	_ = cmd.Flags().MarkHidden("instructions")
	return cmd
}
