// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list projects",
		Example: `# List all stored Aura project configurations
neo4j-cli aura config project list

# Emit JSON for scripting
neo4j-cli aura config project list --format json

# Pipe through jq to print the default project's name
neo4j-cli aura config project list --format json | jq -r '.default'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Aura.GetPrintable("aura-projects"), projectPrintFields)
			return nil
		},
	}
}
