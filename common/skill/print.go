// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

func newPrintCmd(_ *clicfg.Config, bundle fs.FS, _ string) *cobra.Command {
	return &cobra.Command{
		Use:   "print",
		Short: "Print the embedded SKILL.md to stdout",
		Long: "Writes the bundled SKILL.md verbatim to stdout so you can " +
			"preview the skill markdown before running `skill install`. " +
			"The {{VERSION}} placeholder is left literal; substitution " +
			"happens at install time.",
		Example: `# Print the embedded SKILL.md to stdout
neo4j-cli skill print

# Save the embedded SKILL.md to a file for review
neo4j-cli skill print > skill-preview.md

# Print the embedded SKILL.md (--format is accepted for parity but ignored — output is always raw markdown)
neo4j-cli skill print --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			data, err := fs.ReadFile(bundle, "SKILL.md")
			if err != nil {
				return err
			}
			cmd.Print(string(data))
			return nil
		},
	}
}
