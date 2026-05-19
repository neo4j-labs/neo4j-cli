// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewInvokeCmd returns a placeholder `agent invoke` command. The
// implementation is filled in by task-010.
func NewInvokeCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "invoke <id>",
		Short:       "Invoke an agent with an input prompt",
		Example: `# Invoke an agent with a prompt
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --rw

# Invoke an agent in a specific organization and project
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --organization-id 00000000-0000-0000-0000-000000000000 --project-id 00000000-0000-0000-0000-000000000000 --rw

# Invoke an agent and emit the response as JSON
neo4j-cli aura agent invoke 00000000-0000-0000-0000-000000000000 --input "hello" --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
