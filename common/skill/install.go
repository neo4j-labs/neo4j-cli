// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill/catalog"
)

func newInstallCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	var (
		agentFilter string
		installAll  bool
		refresh     bool
	)
	cmd := &cobra.Command{
		Use:         "install [skill-name]",
		Short:       "Install a skill bundle into supported AI agents",
		Annotations: map[string]string{"write": "true"},
		Long: "Without a positional, installs the embedded self-skill into " +
			"every detected agent. With a [skill-name] positional, installs " +
			"that named skill (self-skill or a curated catalog skill from " +
			"github.com/neo4j-contrib/neo4j-skills). Use --all to install " +
			"the self-skill plus every catalog entry, --agent <name> " +
			"(case-insensitive) to scope to one agent, and --refresh to " +
			"force a network fetch of the catalog before installing. " +
			"Passing an agent name as the positional is a hard error — use " +
			"--agent <name> instead." +
			"\n\nSupported agents: " + strings.Join(agentNames(), ", "),
		Example: `# Install the embedded self-skill into every detected agent
neo4j-cli skill install --rw

# Install a curated catalog skill into every detected agent
neo4j-cli skill install neo4j-cypher-skill --rw

# Install the self-skill plus every catalog skill into every detected agent
neo4j-cli skill install --all --rw

# Force a catalog refresh before installing
neo4j-cli skill install neo4j-cypher-skill --refresh --rw

# Install the self-skill into a single agent
neo4j-cli skill install --agent claude-code --rw

# Install and emit the result as JSON (machine-readable)
neo4j-cli skill install --format json --rw`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			skillArg := ""
			if len(args) == 1 {
				skillArg = args[0]
			}

			if installAll && skillArg != "" {
				return fmt.Errorf("--all cannot be combined with a [skill-name] positional")
			}

			return runInstall(cmd, cfg, bundle, skillName, skillArg, agentFilter, installAll, refresh)
		},
	}
	cmd.Flags().StringVar(&agentFilter, "agent", "", "Restrict install to a single agent (case-insensitive). See --help for supported agents.")
	cmd.Flags().BoolVar(&installAll, "all", false, "Install the self-skill plus every curated catalog skill.")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a network refresh of the catalog before installing.")
	return cmd
}

// runInstall dispatches across the install modes (self-only, single
// catalog skill, --all) and threads the auto-refresh policy through
// loadOrRefreshCatalog. Catalog access is gated on whether the current
// invocation needs it (self-only installs and the agent-name hard-break
// skip the catalog entirely).
func runInstall(cmd *cobra.Command, cfg *clicfg.Config, bundle fs.FS, skillName, skillArg, agentFilter string, installAll, refresh bool) error {
	// Agent-name hard-break must fire before any catalog load — the
	// user gets a clean did-you-mean error without a network round-trip.
	if !installAll && skillArg != "" && !catalog.IsReserved(skillArg, skillName) && isAgentName(skillArg) {
		return didYouMeanAgentErr(skillArg)
	}

	needsCatalogContent := installAll || (skillArg != "" && !catalog.IsReserved(skillArg, skillName))
	needsCatalog := needsCatalogContent || refresh

	var cat *catalog.Catalog
	if needsCatalog {
		load, err := loadOrRefreshCatalog(cmd.Context(), cfg, catalogOpts{
			forceRefresh:       refresh,
			requireUsableCache: needsCatalogContent,
		})
		if err != nil {
			return err
		}
		load.PrintWarn(cmd.ErrOrStderr())
		cat = load.Cat
	}

	if installAll {
		return runInstallAll(cmd, cfg, bundle, skillName, agentFilter, cat)
	}

	src, entry, err := resolveSkillSource(bundle, cfg.Version, cat, cfg.Aura.Fs(), skillName, skillArg)
	if err != nil {
		return err
	}

	installDir := skillName
	if entry != nil {
		installDir = entry.Name
	}

	targets, ierr := Install(cfg.Aura.Fs(), src, installDir, agentFilter)
	if ierr != nil {
		return formatAgentErr(ierr)
	}
	renderInstallResult(cmd, cfg, installDir, "installed", targets)
	return nil
}

// runInstallAll installs the self-skill plus every catalog entry into
// the resolved agent set. Per-skill errors are collected so a single bad
// catalog skill does not abort the rest; the aggregate is surfaced as a
// non-zero exit.
func runInstallAll(cmd *cobra.Command, cfg *clicfg.Config, bundle fs.FS, skillName, agentFilter string, cat *catalog.Catalog) error {
	if cat == nil {
		return fmt.Errorf("skill: --all requires a usable catalog cache")
	}

	var allRows []installResultRow
	var failures []string

	selfTargets, err := Install(cfg.Aura.Fs(), Source{FS: bundle, Version: cfg.Version}, skillName, agentFilter)
	if err != nil {
		return formatAgentErr(err)
	}
	allRows = append(allRows, installRowsFor(skillName, "installed", selfTargets)...)

	for _, entry := range cat.Skills {
		if catalog.IsReserved(entry.Name, skillName) {
			continue
		}
		_, sub, lerr := cat.Lookup(cfg.Aura.Fs(), entry.Name, skillName)
		if lerr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Name, lerr))
			continue
		}
		targets, ierr := Install(cfg.Aura.Fs(), Source{FS: sub, Version: cat.Version}, entry.Name, agentFilter)
		if ierr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", entry.Name, formatAgentErr(ierr)))
			continue
		}
		allRows = append(allRows, installRowsFor(entry.Name, "installed", targets)...)
	}

	renderInstallRows(cmd, cfg, allRows, "installed")
	if len(failures) > 0 {
		return fmt.Errorf("skill: %d skill(s) failed to install:\n  - %s", len(failures), strings.Join(failures, "\n  - "))
	}
	return nil
}

// installRowsFor builds the installResultRow slice for one (skill ×
// agents) tuple. Reused by --all so multiple skills can share one
// renderInstallRows call.
func installRowsFor(skillName, action string, targets []*Agent) []installResultRow {
	rows := make([]installResultRow, 0, len(targets))
	for _, a := range targets {
		sp, _ := a.SkillsPath()
		var path string
		if sp != "" {
			path = sp + "/" + skillName
		}
		rows = append(rows, installResultRow{
			Agent:       a.Name,
			DisplayName: a.DisplayName,
			SkillsPath:  path,
			Action:      action,
		})
	}
	return rows
}

// renderInstallRows emits a pre-built row set. Used by --all where the
// rows span multiple skills; the single-skill path keeps using
// renderInstallResult. action is "installed" or "removed".
func renderInstallRows(cmd *cobra.Command, cfg *clicfg.Config, rows []installResultRow, action string) {
	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Printf("No agents to %s.\n", strings.TrimSuffix(action, "ed"))
		return
	}
	data := installResults{rows: rows, action: action}
	commonoutput.PrintBodyMap(cmd, cfg, data, []string{"agent", "display_name", "skills_path", "action"})
}

// installResultRow is the JSON shape emitted by install/remove.
type installResultRow struct {
	Agent       string `json:"agent"`
	DisplayName string `json:"display_name"`
	SkillsPath  string `json:"skills_path"`
	Action      string `json:"action"`
}

// installResults implements common/output.ResponseData for install/remove results.
type installResults struct {
	rows   []installResultRow
	action string
}

// AsArray returns each row as a column-keyed map for table rendering.
func (r installResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r.rows))
	for _, row := range r.rows {
		out = append(out, map[string]any{
			"agent":        row.Agent,
			"display_name": row.DisplayName,
			"skills_path":  row.SkillsPath,
			"action":       row.Action,
		})
	}
	return out
}

// MarshalJSON delegates to default slice marshalling, preserving the
// existing JSON array-of-objects shape.
func (r installResults) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.rows)
}

// renderInstallResult prints the install/remove outcome as a table or
// JSON. `action` is "installed" or "removed" — printed in the Action
// column / JSON field. Empty target list emits a friendly note in table
// mode and an empty array in JSON mode.
func renderInstallResult(cmd *cobra.Command, cfg *clicfg.Config, skillName, action string, targets []*Agent) {
	rows := installRowsFor(skillName, action, targets)
	data := installResults{rows: rows, action: action}

	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Printf("No agents to %s.\n", strings.TrimSuffix(action, "ed"))
		return
	}

	commonoutput.PrintBodyMap(cmd, cfg, data, []string{"agent", "display_name", "skills_path", "action"})
}
