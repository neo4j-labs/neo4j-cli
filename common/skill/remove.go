// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/skill/catalog"
)

func newRemoveCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	var agentFilter string
	cmd := &cobra.Command{
		Use:         "remove <skill-name>",
		Short:       "Remove an installed skill bundle",
		Annotations: map[string]string{"write": "true"},
		Long: "Removes the named skill (self-skill or catalog skill) from " +
			"every detected agent. Use --agent <name> (case-insensitive) to " +
			"scope the removal to one agent. Passing an agent name as the " +
			"positional is a hard error — use --agent <name> instead. " +
			"Idempotent: a second run on a clean target is a no-op." +
			"\n\nSupported agents: " + strings.Join(agentNames(), ", "),
		Example: `# Remove the self-skill from every detected agent
neo4j-cli skill remove self --rw

# Remove the self-skill from a single agent
neo4j-cli skill remove self --agent claude-code --rw

# Remove and emit the result as JSON (machine-readable)
neo4j-cli skill remove self --format json --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			skillArg := args[0]

			if err := validateRemoveSkillName(skillName, skillArg); err != nil {
				return err
			}

			targets, err := Remove(cfg.Aura.Fs(), skillName, agentFilter)
			if err != nil {
				return formatAgentErr(err)
			}
			renderInstallResult(cmd, cfg, skillName, "removed", targets)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFilter, "agent", "", "Restrict remove to a single agent (case-insensitive). See --help for supported agents.")
	return cmd
}

// validateRemoveSkillName mirrors install's positional validator but
// requires an explicit arg (no default to self). Reserved self/binary
// names pass; agent-name positional hard-breaks; anything else is
// unknown. Catalog Lookup will plug in here in task-008.
func validateRemoveSkillName(binaryName, skillArg string) error {
	if catalog.IsReserved(skillArg, binaryName) {
		return nil
	}
	if isAgentName(skillArg) {
		return didYouMeanAgentErr(skillArg)
	}
	return unknownSkillErr(skillArg)
}
