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
