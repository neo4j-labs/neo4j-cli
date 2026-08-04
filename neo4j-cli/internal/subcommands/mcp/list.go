// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"encoding/json"

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
	InstalledVersion string `json:"installed_version"`
}

// mcpListResults implements output.ResponseData for mcp list.
type mcpListResults []mcpListResultRow

func (r mcpListResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"agent":             row.Agent,
			"display_name":      row.DisplayName,
			"detected":          boolStr(row.Detected),
			"installed":         boolStr(row.Installed),
			"installed_version": row.InstalledVersion,
		})
	}
	return out
}

func (r mcpListResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]mcpListResultRow(r))
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
			"and installed_version (the binary path from the config entry, if any).",
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

	rows := make(mcpListResults, 0, len(installs))
	for _, inst := range installs {
		rows = append(rows, mcpListResultRow{
			Agent:            inst.Agent.Name,
			DisplayName:      inst.Agent.DisplayName,
			Detected:         inst.Detected,
			Installed:        inst.Installed,
			InstalledVersion: inst.InstalledVersion,
		})
	}

	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Println("No MCP-capable agents found.")
		return nil
	}
	commonoutput.PrintBodyMap(cmd, cfg, rows, []string{"agent", "display_name", "detected", "installed", "installed_version"})
	return nil
}
