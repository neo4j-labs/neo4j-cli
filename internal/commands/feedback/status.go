// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

type statusObj struct {
	Enabled bool `json:"enabled"`
	// DoNotTrack is true when DO_NOT_TRACK=1 overrides everything else.
	DoNotTrack bool `json:"do_not_track"`
	// TelemetryOff is true when the CLI's telemetry setting is off, which
	// also switches feedback off.
	TelemetryOff    bool   `json:"telemetry_off"`
	Endpoint        string `json:"endpoint"`
	AuthorizedUntil string `json:"authorized_until"`
	ReceiptsPath    string `json:"receipts_path"`
	// Instructions is the server-instructions variant set at the last `enable`.
	Instructions string `json:"instructions,omitempty"`
	// RegisteredAgents lists agents that have the feedback tool registered.
	RegisteredAgents []string `json:"registered_agents"`
	// Notes carries operator-facing follow-ups, such as a manual command.
	Notes []string `json:"notes,omitempty"`
}

// statusFields are the table columns for the field/value rows of AsArray.
var statusFields = []string{"field", "value"}

// AsArray renders the state as one field/value row per entry, for the table
// view; json and toon use the struct's own tags.
func (s statusObj) AsArray() []map[string]any {
	rows := []map[string]any{
		{"field": "enabled", "value": s.Enabled},
		{"field": "do_not_track", "value": s.DoNotTrack},
		{"field": "telemetry_off", "value": s.TelemetryOff},
		{"field": "endpoint", "value": s.Endpoint},
		{"field": "authorized_until", "value": s.AuthorizedUntil},
		{"field": "receipts_path", "value": s.ReceiptsPath},
		{"field": "registered_agents", "value": strings.Join(s.RegisteredAgents, ", ")},
	}
	if s.Instructions != "" {
		rows = append(rows, map[string]any{"field": "instructions", "value": s.Instructions})
	}
	for _, n := range s.Notes {
		rows = append(rows, map[string]any{"field": "note", "value": n})
	}
	return rows
}

func currentStatus(cfg *clicfg.Config) statusObj {
	return statusObj{
		Enabled:          Authorized(cfg, now()) && optOut(cfg) == "",
		DoNotTrack:       doNotTrack(),
		TelemetryOff:     telemetryOff(cfg),
		Endpoint:         endpoint(cfg),
		AuthorizedUntil:  getString(cfg, keyAuthorizedUntil),
		ReceiptsPath:     receiptsPath(),
		Instructions:     getString(cfg, keyInstructions),
		RegisteredAgents: []string{},
	}
}

func newStatusCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether feedback sending is enabled",
		Long:  "Show whether feedback sending is currently authorized, where reports would go, when authorization expires, and where local receipts are kept.",
		Example: `# Show the feedback state
neo4j-cli feedback status

# Show the feedback state as JSON
neo4j-cli feedback status --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			st := currentStatus(cfg)
			st.RegisteredAgents = registeredAgents(cmd.Context())
			return writeStatus(cmd, cfg, st)
		},
	}
}
