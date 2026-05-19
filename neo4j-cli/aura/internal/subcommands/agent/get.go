// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewGetCmd returns a placeholder `agent get` command. The implementation
// is filled in by task-005.
func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Returns agent details",
		Example: `# Get details for an agent
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000

# Get an agent in a specific organization and project
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000

# Get agent details as JSON for scripting
neo4j-cli aura agent get 00000000-0000-0000-0000-000000000000 --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
