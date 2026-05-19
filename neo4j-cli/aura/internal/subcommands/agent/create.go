// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCreateCmd returns a placeholder `agent create` command. The
// implementation is filled in by task-006.
func NewCreateCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Create a new agent",
		Example: `# Create an agent in the default project
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw

# Create an agent with a system prompt
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --system-prompt "you are helpful" --rw

# Create an agent and emit the response as JSON
neo4j-cli aura agent create --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
