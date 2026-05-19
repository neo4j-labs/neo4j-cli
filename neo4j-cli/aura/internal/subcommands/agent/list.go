// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewListCmd returns a placeholder `agent list` command. The implementation
// is filled in by task-004; this scaffold lets the parent compile and lets
// `--help` enumerate the subtree.
func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Returns all agents",
		Example: `# List all agents in the default project
neo4j-cli aura agent list

# List agents in a specific organization and project
neo4j-cli aura agent list --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# List agents as JSON for scripting
neo4j-cli aura agent list --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
