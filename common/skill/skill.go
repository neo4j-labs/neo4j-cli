// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
)

// NewCmd builds the per-binary `skill` cobra command tree. `bundle` is the
// embedded SKILL.md + references/ tree for the calling binary; `skillName`
// is the lowercase id used as the on-disk install dir
// (`<agentSkillsDir>/<skillName>/`) and the SKILL.md frontmatter `name:`.
//
// Each leaf renders results as a table by default and emits a JSON envelope
// when `--output json` is passed (matching neo4j-cli/aura/internal/output
// conventions).
func NewCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Install agent skills for this CLI into supported AI agents",
		Long: "Install, remove, list, and check the per-binary agent-skill " +
			"bundle. The bundle teaches AI agents (Claude Code, Cursor, " +
			"Windsurf, etc.) how to drive this CLI.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			outputValue := cmd.Flags().Lookup("output").Value.String()
			if outputValue != "" {
				valid := false
				for _, v := range clicfg.ValidOutputValues {
					if v == outputValue {
						valid = true
						break
					}
				}
				if !valid {
					return clierr.NewUsageError("invalid output value specified: %s", outputValue)
				}
			}
			cfg.Aura.BindOutput(cmd.Flags().Lookup("output"))
			return nil
		},
	}

	cmd.PersistentFlags().String("output", "", fmt.Sprintf("Format to print console output in, from a choice of [%s]", strings.Join(clicfg.ValidOutputValues[:], ", ")))

	cmd.AddCommand(newInstallCmd(cfg, bundle, skillName))
	cmd.AddCommand(newRemoveCmd(cfg, skillName))
	cmd.AddCommand(newListCmd(cfg, skillName))
	cmd.AddCommand(newCheckCmd(cfg, skillName))

	return cmd
}
