// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/output"
	"github.com/spf13/cobra"
)

var credentialFields = []string{"name", "type", "identifier", "default"}

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list credentials",
		Example: `# List all stored Aura credentials (the default column flags the active one)
neo4j-cli aura credential list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli aura credential list --format json

# Pipe through jq to print just the default credential's name
neo4j-cli aura credential list --format json | jq -r '.data[] | select(.default == true) | .name'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			output.PrintBodyMap(cmd, cfg, cfg.Credentials.Aura.Printable(), credentialFields)
			return nil
		},
	}
}
