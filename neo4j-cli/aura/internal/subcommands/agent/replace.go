// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewReplaceCmd returns a placeholder `agent replace` command. The
// implementation is filled in by task-008.
func NewReplaceCmd(cfg *clicfg.Config) *cobra.Command {
	_ = cfg
	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "replace <id>",
		Short:       "Replace an existing agent",
		Example: `# Replace an agent's full definition
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw

# Replace an agent with a system prompt
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --system-prompt "you are helpful" --rw

# Replace an agent and emit the response as JSON
neo4j-cli aura agent replace 00000000-0000-0000-0000-000000000000 --name my-agent --description "demo" --dbid 00000000-0000-0000-0000-000000000000 --tools "[]" --rw --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return cmd
}
