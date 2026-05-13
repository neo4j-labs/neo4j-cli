// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:         "remove <name>",
		Short:       "Removes a project",
		Annotations: map[string]string{"write": "true"},
		Example: `# Remove a stored project configuration by name
neo4j-cli aura config project remove prod --rw

# Remove a staging project
neo4j-cli aura config project remove staging --rw

# Remove and confirm by listing remaining projects as JSON
neo4j-cli aura config project remove prod --rw && neo4j-cli aura config project list --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Aura.Projects.Remove(args[0])
		},
	}
}
