// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	var agentFilter string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove neo4j-cli MCP server configuration",
		Long: "Removes the neo4j-cli MCP server entry from the specified agent's " +
			"config file. Use --agent <name> (case-insensitive) to scope the removal " +
			"to one agent; omit to remove from every detected MCP-capable agent. " +
			"Idempotent: re-running on an already-removed entry exits zero. " +
			"\n\nSupported agents: " + strings.Join(skill.MCPAgentNames(), ", "),
		Example: `# Remove neo4j-cli from every detected MCP agent
	neo4j-cli mcp remove --rw

	# Remove from a single agent
	neo4j-cli mcp remove --agent claude-desktop --rw

	# Remove and emit the result as JSON
	neo4j-cli mcp remove --agent claude-desktop --format json --rw`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"write": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runRemoveCmd(cfg, cmd, agentFilter)
		},
	}

	cmd.Flags().StringVar(&agentFilter, "agent", "", "Restrict remove to a single MCP agent (case-insensitive). See --help for supported agents.")
	return cmd
}

func runRemoveCmd(cfg *clicfg.Config, cmd *cobra.Command, agentFilter string) error {
	fs := cfg.Aura.Fs()

	var targets []*skill.Agent
	if agentFilter != "" {
		a := skill.FindAgent(agentFilter)
		if a == nil {
			return clierr.NewUsageError("unknown MCP agent: %q\nvalid agents: %s", agentFilter, strings.Join(skill.MCPAgentNames(), ", "))
		}
		if !a.SupportsMCP() {
			return clierr.NewUsageError("%q is a skill-only agent; use 'skill remove' instead", a.Name)
		}
		targets = []*skill.Agent{a}
	} else {
		targets = skill.DetectMCPAgents(fs)
	}

	var rows resultRows[installResult]
	for _, a := range targets {
		if err := removeFromOne(fs, a); err != nil {
			return err
		}
		rows = append(rows, installResult{
			Agent:       a.Name,
			DisplayName: a.DisplayName,
			Method:      "config",
		})
	}

	renderRemoveResults(cmd, cfg, rows)
	return nil
}

func removeFromOne(fs afero.Fs, a *skill.Agent) error {
	return skill.RemoveMCPConfig(fs, a)
}

func renderRemoveResults(cmd *cobra.Command, cfg *clicfg.Config, rows resultRows[installResult]) {
	if len(rows) == 0 {
		// No agents to remove from — either none detected or none left. Not an
		// error (idempotent), but a friendly note when table output.
		return
	}
	commonoutput.PrintBodyMap(cmd, cfg, rows, []string{"agent", "display_name", "method"})
}
