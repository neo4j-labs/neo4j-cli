// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

func newPrintCmd(_ *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:   "print [skill-name]",
		Short: "Print a skill's SKILL.md to stdout",
		Long: "Writes the SKILL.md for the named skill verbatim to stdout. " +
			"Defaults to the embedded self-skill when no positional is " +
			"supplied. The {{VERSION}} placeholder in the self-skill bundle " +
			"is left literal; substitution happens at install time. Passing " +
			"an agent name as the positional is a hard error — use the " +
			"--agent flag on install/remove instead.",
		Example: `# Print the embedded self-skill SKILL.md to stdout
neo4j-cli skill print

# Print the self-skill explicitly by canonical name
neo4j-cli skill print self

# Save the embedded SKILL.md to a file for review
neo4j-cli skill print > skill-preview.md`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			skillArg := ""
			if len(args) == 1 {
				skillArg = args[0]
			}

			src, err := resolveSkillSource(bundle, "", skillName, skillArg)
			if err != nil {
				return err
			}
			// Print never injects a version — keep the bundled
			// frontmatter (incl. {{VERSION}} placeholder) verbatim.
			data, rerr := fs.ReadFile(src.FS, "SKILL.md")
			if rerr != nil {
				if errors.Is(rerr, fs.ErrNotExist) {
					return errors.New("skill: SKILL.md not found in bundle")
				}
				return rerr
			}
			cmd.Print(string(data))
			return nil
		},
	}
}
