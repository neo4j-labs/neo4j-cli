// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewDeleteCmd returns a placeholder `agent delete` command. The
// implementation is filled in by task-009.
func NewDeleteCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "delete <id>",
		Short:       "Delete the given agent",
		Example: `# Delete an agent by ID
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw

# Delete an agent in a specific organization and project
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Delete an agent and emit the response as JSON
neo4j-cli aura agent delete 00000000-0000-0000-0000-000000000000 --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
