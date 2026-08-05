// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

func newToolCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "List the MCP tools the server registers",
		Long: "List the MCP tools the server registers, without starting a transport, so you can " +
			"inspect the surface an MCP client will see before installing the connector. " +
			"The table shows each tool's name, title, read_only_hint and destructive_hint; " +
			"--format json|toon adds the full description plus the idempotent_hint and " +
			"open_world_hint behaviour hints clients use to decide whether a call needs " +
			"confirmation.",
		Example: `# List the registered MCP tools
neo4j-cli mcp tool

# Inspect the full tool manifest, including descriptions and hints
neo4j-cli mcp tool --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			commonoutput.PrintBodyMap(cmd, cfg, toolRows(toolDefinitions()),
				[]string{"name", "title", "read_only_hint", "destructive_hint", "description"})
			return nil
		},
	}

	return cmd
}
