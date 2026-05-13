// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists the current global configuration values",
		Example: `# List all configuration values as a table
neo4j-cli config list

# List all configuration values as JSON (machine-readable)
neo4j-cli config list --format json

# List all configuration values in toon format
neo4j-cli config list --format toon`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			output.PrintBodyMap(cmd, cfg, cfg.Printable(), configPrintFields)
		},
	}
}
