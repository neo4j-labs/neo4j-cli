// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill"
	"github.com/spf13/cobra"
)

// mcpListResultRow is the JSON shape emitted by mcp list.
type mcpListResultRow struct {
	Agent            string `json:"agent"`
	DisplayName      string `json:"display_name"`
	Detected         bool   `json:"detected"`
	Installed        bool   `json:"installed"`
	InstalledCommand string `json:"installed_command"`
}

func (r mcpListResultRow) asArrayRow() map[string]any {
	return map[string]any{
		"agent":             r.Agent,
		"display_name":      r.DisplayName,
		"detected":          boolStr(r.Detected),
		"installed":         boolStr(r.Installed),
		"installed_command": r.InstalledCommand,
	}
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MCP-capable agents and their install state",
		Long: "Lists every MCP-capable agent and whether neo4j-cli is installed " +
			"as an MCP server for each. Columns: agent, display_name, detected " +
			"(agent directory exists), installed (neo4j-cli entry in config), " +
			"and installed_command (the binary path from the config entry, if any).",
		Example: `# List MCP agents and their install state (table)
neo4j-cli mcp list

# List as JSON (machine-readable)
neo4j-cli mcp list --format json

# List in toon format
neo4j-cli mcp list --format toon`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runListCmd(cfg, cmd)
		},
	}
	return cmd
}

func runListCmd(cfg *clicfg.Config, cmd *cobra.Command) error {
	fs := cfg.Aura.Fs()
	installs := skill.MCPList(fs)

	rows := make(resultRows[mcpListResultRow], 0, len(installs))
	for _, inst := range installs {
		rows = append(rows, mcpListResultRow{
			Agent:            inst.Agent.Name,
			DisplayName:      inst.Agent.DisplayName,
			Detected:         inst.Detected,
			Installed:        inst.Installed,
			InstalledCommand: inst.InstalledVersion,
		})
	}

	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Println("No MCP-capable agents found.")
		return nil
	}
	commonoutput.PrintBodyMap(cmd, cfg, rows, []string{"agent", "display_name", "detected", "installed", "installed_command"})
	return nil
}
