// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill"
	"github.com/spf13/cobra"
)

// mcpCheckResultRow is the JSON shape emitted by mcp check.
type mcpCheckResultRow struct {
	Agent             string `json:"agent"`
	InstalledCommand  string `json:"installed_command"`
	CurrentCommand    string `json:"current_command"`
	HasMCPEnvManifest bool   `json:"has_mcp_env_manifest"`
	Status            string `json:"status"`
}

func (r mcpCheckResultRow) asArrayRow() map[string]any {
	return map[string]any{
		"agent":                r.Agent,
		"installed_command":    r.InstalledCommand,
		"current_command":      r.CurrentCommand,
		"has_mcp_env_manifest": r.HasMCPEnvManifest,
		"status":               r.Status,
	}
}

func newCheckCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check installed MCP servers for drift",
		Long: "Inspects every MCP-capable agent and compares the neo4j-cli " +
			"server entry's command path and env block against the current binary. " +
			"Columns: agent, installed_command (path in config), current_command (this " +
			"binary's path), has_mcp_env_manifest (whether the env block has the " +
			"manifest marker), status where status is ok | drift | not-installed. " +
			"Drift includes a changed binary path or a missing manifest env marker. " +
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
	var rows resultRows[mcpCheckResultRow]

	for _, inst := range installs {
		if !inst.Installed {
			continue
		}
		status := "ok"
		if inst.InstalledVersion != currentBin || !inst.InstalledHasMCPManifest {
			status = "drift"
		}
		rows = append(rows, mcpCheckResultRow{
			Agent:             inst.Agent.Name,
			InstalledCommand:  inst.InstalledVersion,
			CurrentCommand:    currentBin,
			HasMCPEnvManifest: inst.InstalledHasMCPManifest,
			Status:            status,
		})
	}

	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Println("No installed MCP servers found.")
		return nil
	}

	commonoutput.PrintBodyMap(cmd, cfg, rows, []string{"agent", "installed_command", "current_command", "has_mcp_env_manifest", "status"})

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
