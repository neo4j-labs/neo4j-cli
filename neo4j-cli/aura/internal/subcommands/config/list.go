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
		Short: "Lists the current configuration of the Aura CLI subcommand",
		Example: `# List the current Aura CLI configuration
neo4j-cli aura config list

# Emit configuration as JSON for scripting
neo4j-cli aura config list --format json

# Pipe through jq to print just the default-context value
neo4j-cli aura config list --format json | jq -r '."default-context"'`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			output.PrintBodyMap(cmd, cfg, cfg.Printable(), configPrintFields)
		},
	}
}
