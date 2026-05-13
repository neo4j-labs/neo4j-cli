// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

func newRemoveCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:         "remove [agent]",
		Short:       "Remove the installed skill bundle",
		Annotations: map[string]string{"write": "true"},
		Long: "Without an argument, removes from every detected agent. " +
			"With an [agent] argument (case-insensitive), removes from that " +
			"single agent. Idempotent: a second run on a clean target is a " +
			"no-op." +
			"\n\nSupported agents: " + strings.Join(agentNames(), ", "),
		Example: `# Remove the skill from every detected agent
neo4j-cli skill remove --rw

# Remove the skill from a single agent (case-insensitive name)
neo4j-cli skill remove claude-code --rw

# Remove and emit the result as JSON (machine-readable)
neo4j-cli skill remove --format json --rw`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			targets, err := Remove(cfg.Aura.Fs(), skillName, filter)
			if err != nil {
				return formatAgentErr(err)
			}
			renderInstallResult(cmd, cfg, skillName, "removed", targets)
			return nil
		},
	}
}
