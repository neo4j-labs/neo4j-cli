// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/skill/catalog"
)

func newPrintCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:   "print [skill-name]",
		Short: "Print a skill's SKILL.md to stdout",
		Long: "Writes the SKILL.md for the named skill verbatim to stdout. " +
			"Defaults to the embedded self-skill when no positional is " +
			"supplied; pass 'self' (or the binary-name alias) for the same " +
			"effect. Pass a curated catalog skill name to print its cached " +
			"SKILL.md — print is offline-only and will not fetch the " +
			"catalog. Run 'neo4j-cli skill refresh' first if the catalog " +
			"cache is missing. The {{VERSION}} placeholder in the self-skill " +
			"bundle is left literal; substitution happens at install time. " +
			"Passing an agent name as the positional is a hard error — use " +
			"the --agent flag on install/remove instead.",
		Example: `# Print the embedded self-skill SKILL.md to stdout
neo4j-cli skill print

# Print the self-skill explicitly by canonical name
neo4j-cli skill print self

# Print a curated catalog skill's cached SKILL.md
neo4j-cli skill print neo4j-cypher-skill

# Save the embedded SKILL.md to a file for review
neo4j-cli skill print > skill-preview.md`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			skillArg := ""
			if len(args) == 1 {
				skillArg = args[0]
			}
			return runPrint(cmd, cfg, bundle, skillName, skillArg)
		},
	}
}

// runPrint resolves the requested skill to its SKILL.md and writes it to
// stdout verbatim. Self-skill (empty arg / `self` / binary-name alias)
// reads from the embedded bundle; catalog skill names read from the on-
// disk cache via catalog.Load — print never hits the network (REQ-NF-008).
func runPrint(cmd *cobra.Command, cfg *clicfg.Config, bundle fs.FS, skillName, skillArg string) error {
	cacheRoot, cerr := catalogCacheRootFn()
	if cerr != nil {
		return fmt.Errorf("skill: resolve cache root: %w", cerr)
	}
	cat, lerr := catalog.Load(cfg.Aura.Fs(), cacheRoot)
	if lerr != nil {
		cat = nil
	}

	src, _, err := resolveSkillSource(bundle, cfg.Version, cat, cfg.Aura.Fs(), skillName, skillArg)
	if err != nil {
		if cat == nil && skillArg != "" && !catalog.IsReserved(skillArg, skillName) && !isAgentName(skillArg) {
			return clierr.NewUsageError("unknown skill: %s; %s", skillArg, coldCacheHint(skillName))
		}
		return err
	}
	data, rerr := fs.ReadFile(src.FS, "SKILL.md")
	if rerr != nil {
		if errors.Is(rerr, fs.ErrNotExist) {
			return errors.New("skill: SKILL.md not found in bundle")
		}
		return rerr
	}
	cmd.Print(string(data))
	return nil
}
