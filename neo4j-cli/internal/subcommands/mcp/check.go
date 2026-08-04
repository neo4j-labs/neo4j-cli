// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"encoding/json"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill"
	"github.com/spf13/cobra"
)

// mcpCheckResultRow is the JSON shape emitted by mcp check.
type mcpCheckResultRow struct {
	Agent            string `json:"agent"`
	InstalledVersion string `json:"installed_version"`
	CurrentVersion   string `json:"current_version"`
	Status           string `json:"status"`
}

// mcpCheckResults implements output.ResponseData for mcp check.
type mcpCheckResults []mcpCheckResultRow

func (r mcpCheckResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"agent":             row.Agent,
			"installed_version": row.InstalledVersion,
			"current_version":   row.CurrentVersion,
			"status":            row.Status,
		})
	}
	return out
}

func (r mcpCheckResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]mcpCheckResultRow(r))
}

func newCheckCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check installed MCP servers for drift",
		Long: "Inspects every MCP-capable agent and compares the neo4j-cli " +
			"server entry's command path against the current binary path. Columns: " +
			"agent, installed_version (path in config), current_version (this binary's " +
			"path), status where status is ok | drift | not-installed. " +
			"Exits non-zero when any installed entry has drifted.",
		Example: `# Check MCP install state for drift (table)
	neo4j-cli mcp check

	# Check as JSON (machine-readable)
	neo4j-cli mcp check --format json

	# Check in toon format
	neo4j-cli mcp check --format toon`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runCheckCmd(cfg, cmd)
		},
	}
	return cmd
}

func runCheckCmd(cfg *clicfg.Config, cmd *cobra.Command) error {
	fs := cfg.Aura.Fs()
	currentBin, err := os.Executable()
	if err != nil {
		return clierr.NewFatalError("cannot resolve binary path: %s", err.Error())
	}

	installs := skill.MCPList(fs)
	var rows mcpCheckResults

	for _, inst := range installs {
		if !inst.Installed {
			continue
		}
		status := "ok"
		if inst.InstalledVersion != currentBin {
			status = "drift"
		}
		rows = append(rows, mcpCheckResultRow{
			Agent:            inst.Agent.Name,
			InstalledVersion: inst.InstalledVersion,
			CurrentVersion:   currentBin,
			Status:           status,
		})
	}

	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Println("No installed MCP servers found.")
		return nil
	}

	commonoutput.PrintBodyMap(cmd, cfg, rows, []string{"agent", "installed_version", "current_version", "status"})

	drift := 0
	for _, r := range rows {
		if r.Status == "drift" {
			drift++
		}
	}
	if drift > 0 {
		return clierr.NewValidationError("MCP: drift detected in %d agent(s) — run `neo4j-cli mcp install` to refresh", drift)
	}
	return nil
}
