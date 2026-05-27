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
	src, err := resolvePrintSource(cfg, bundle, skillName, skillArg)
	if err != nil {
		return err
	}
	data, rerr := fs.ReadFile(src, "SKILL.md")
	if rerr != nil {
		if errors.Is(rerr, fs.ErrNotExist) {
			return errors.New("skill: SKILL.md not found in bundle")
		}
		return rerr
	}
	cmd.Print(string(data))
	return nil
}

// resolvePrintSource maps the positional skill-name to an fs.FS rooted at
// the SKILL.md to print. Empty / self / binary-alias → embedded bundle.
// Otherwise consults the cached catalog (no network). Cold cache yields a
// usage error pointing at `skill refresh`; agent-name collisions yield the
// shared did-you-mean-agent hard-break.
func resolvePrintSource(cfg *clicfg.Config, bundle fs.FS, skillName, skillArg string) (fs.FS, error) {
	if skillArg == "" {
		return bundle, nil
	}

	src, err := ResolveSelf(bundle, cfg.Version, skillName, skillArg)
	if err == nil {
		return src.FS, nil
	}
	if !errors.Is(err, ErrNotSelfSkill) {
		return nil, err
	}

	cacheRoot, cerr := catalogCacheRootFn()
	if cerr != nil {
		return nil, fmt.Errorf("skill: resolve cache root: %w", cerr)
	}
	cat, lerr := catalog.Load(cfg.Aura.Fs(), cacheRoot)
	if lerr == nil {
		_, sub, lookupErr := cat.Lookup(cfg.Aura.Fs(), skillArg, skillName)
		if lookupErr == nil {
			return sub, nil
		}
	}

	if isAgentName(skillArg) {
		return nil, didYouMeanAgentErr(skillArg)
	}
	if lerr != nil {
		return nil, clierr.NewUsageError(
			"unknown skill: %s; skill catalog cache is empty — run 'neo4j-cli skill refresh' once you have network connectivity",
			skillArg,
		)
	}
	return nil, unknownSkillErr(skillArg)
}
