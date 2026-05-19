// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewUpdateCmd returns a placeholder `agent update` command. The
// implementation is filled in by task-007.
func NewUpdateCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "update <id>",
		Short:       "Partially update an existing agent",
		Example: `# Rename an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --name my-renamed-agent --rw

# Disable an agent
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --enabled=false --rw

# Update an agent and emit the response as JSON
neo4j-cli aura agent update 00000000-0000-0000-0000-000000000000 --description "updated" --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
