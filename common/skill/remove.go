// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/skill/catalog"
)

func newRemoveCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	var (
		agentFilter string
		removeAll   bool
	)
	cmd := &cobra.Command{
		Use:         "remove [skill-name]",
		Short:       "Remove an installed skill bundle",
		Annotations: map[string]string{"write": "true"},
		Long: "Removes the named skill (self-skill or catalog skill) from " +
			"every detected agent. Use --agent <name> (case-insensitive) to " +
			"scope the removal to one agent. Use --all to remove every " +
			"curated catalog skill from every detected agent — the embedded " +
			"self-skill is preserved. Passing 'self' (or the binary-name " +
			"alias) removes the self-skill and prints a reinstall hint. " +
			"Idempotent: a name with no installation present exits zero. " +
			"Passing an agent name as the positional is a hard error — use " +
			"--agent <name> instead. --all reads only the cached catalog; " +
			"with no cache it is a no-op." +
			"\n\nSupported agents: " + strings.Join(agentNames(), ", "),
		Example: `# Remove the self-skill from every detected agent
neo4j-cli skill remove self --rw

# Remove a curated catalog skill from every detected agent
neo4j-cli skill remove neo4j-cypher-skill --rw

# Remove every catalog skill (self-skill is preserved)
neo4j-cli skill remove --all --rw

# Remove the self-skill from a single agent
neo4j-cli skill remove self --agent claude-code --rw

# Remove and emit the result as JSON (machine-readable)
neo4j-cli skill remove self --format json --rw`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			skillArg := ""
			if len(args) == 1 {
				skillArg = args[0]
			}

			if removeAll && skillArg != "" {
				return fmt.Errorf("--all cannot be combined with a [skill-name] positional")
			}
			if !removeAll && skillArg == "" {
				return fmt.Errorf("requires a [skill-name] positional or --all")
			}

			return runRemove(cmd, cfg, bundle, skillName, skillArg, agentFilter, removeAll)
		},
	}
	cmd.Flags().StringVar(&agentFilter, "agent", "", "Restrict remove to a single agent (case-insensitive). See --help for supported agents.")
	cmd.Flags().BoolVar(&removeAll, "all", false, "Remove every curated catalog skill (self-skill preserved).")
	return cmd
}

// runRemove dispatches across the remove modes (single self/binary alias,
// single catalog skill, --all). Single-skill removal is idempotent and
// requires no catalog access for the self-skill; catalog skill names are
// validated against the cached plugin.json so an unknown name hard-fails
// instead of silently no-op'ing.
func runRemove(cmd *cobra.Command, cfg *clicfg.Config, bundle fs.FS, skillName, skillArg, agentFilter string, removeAll bool) error {
	if removeAll {
		return runRemoveAll(cmd, cfg, skillName, agentFilter)
	}

	cacheRoot, err := catalogCacheRootFn()
	if err != nil {
		return fmt.Errorf("skill: resolve cache root: %w", err)
	}
	cat := catalog.New(catalog.Options{CacheRoot: cacheRoot, BinaryVersion: cfg.Version})
	if lerr := cat.Load(cfg.Aura.Fs()); lerr != nil {
		cat = nil
	}

	_, entry, rerr := resolveSkillSource(bundle, cfg.Version, cat, cfg.Aura.Fs(), skillName, skillArg)
	if rerr != nil {
		return rerr
	}

	target := skillName
	if entry != nil {
		target = entry.Name
	}

	targets, ierr := Remove(cfg.Aura.Fs(), target, agentFilter)
	if ierr != nil {
		return formatAgentErr(ierr)
	}
	renderInstallResult(cmd, cfg, target, "removed", targets)
	if entry == nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Run '%s skill install' to reinstall.\n", skillName)
	}
	return nil
}

// runRemoveAll removes every catalog skill listed in the cached
// plugin.json from every detected agent (or one when --agent is set). The
// self-skill is never touched (REQ-F-022) — the omission is noted in the
// help text and surfaced as a stderr info line. A cold catalog cache is
// a no-op (nothing to enumerate). An unknown --agent filter is rejected
// up front so the user sees one clean error instead of one per skill.
func runRemoveAll(cmd *cobra.Command, cfg *clicfg.Config, skillName, agentFilter string) error {
	if agentFilter != "" && FindAgent(agentFilter) == nil {
		return formatAgentErr(fmt.Errorf("%w: %q", ErrUnknownAgent, agentFilter))
	}

	cacheRoot, err := catalogCacheRootFn()
	if err != nil {
		return fmt.Errorf("skill: resolve cache root: %w", err)
	}
	cat := catalog.New(catalog.Options{CacheRoot: cacheRoot, BinaryVersion: cfg.Version})
	if lerr := cat.Load(cfg.Aura.Fs()); lerr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skill catalog cache is empty; nothing to remove\n")
		renderInstallRows(cmd, cfg, nil, "removed")
		return nil
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: --all preserves the embedded self-skill (%q); remove it explicitly with '%s skill remove self'\n", skillName, skillName)

	var allRows []installResultRow
	var failures []string
	for _, entry := range cat.Skills {
		if catalog.IsReserved(entry.Name, skillName) {
			continue
		}
		targets, rerr := Remove(cfg.Aura.Fs(), entry.Name, agentFilter)
		if rerr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Name, formatAgentErr(rerr)))
			continue
		}
		allRows = append(allRows, installRowsFor(entry.Name, "removed", targets)...)
	}

	renderInstallRows(cmd, cfg, allRows, "removed")
	if len(failures) > 0 {
		return fmt.Errorf("skill: %d skill(s) failed to remove:\n  - %s", len(failures), strings.Join(failures, "\n  - "))
	}
	return nil
}
