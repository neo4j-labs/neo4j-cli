// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Short:       "Sets the default credential to be used",
		Annotations: map[string]string{"write": "true"},
		Example: `# Switch the default credential used by subsequent aura commands
neo4j-cli aura credential use my-creds --rw

# Switch to a staging credential before running write operations
neo4j-cli aura credential use staging --rw

# Switch and verify the default has been set
neo4j-cli aura credential use my-creds --rw && neo4j-cli aura credential list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Aura.SetDefault(args[0])
		},
	}
}
