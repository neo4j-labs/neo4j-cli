// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
)

func newCheckCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check installed skills for version drift",
		Long: "Inspects every installed skill across detected agents and " +
			"compares its frontmatter `version:` against the source " +
			"version (binary version for the self-skill, the skill's own " +
			"SKILL.md `version:` for catalog skills). Columns: skill, agent, " +
			"installed_version, current_version, status where status ∈ " +
			"ok | drift | unknown-version. Exits non-zero when any row " +
			"is drift or unknown-version. Auto-refreshes the catalog " +
			"cache on 24h staleness when network is available; --refresh " +
			"forces a fetch.",
		Example: `# Check installed skills for version drift (table)
neo4j-cli skill check

# Check installed skills as JSON (machine-readable)
neo4j-cli skill check --format json

# Check installed skills in toon format
neo4j-cli skill check --format toon

# Force a catalog refresh before checking
neo4j-cli skill check --refresh`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runCheck(cmd, cfg, skillName, refresh)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a network refresh of the catalog before checking.")
	return cmd
}

// runCheck loads the catalog (auto-refresh + soft fallback), builds the
// inventory, filters to installed rows, renders them, and exits non-zero
// when any installed row reports drift or unknown-version.
func runCheck(cmd *cobra.Command, cfg *clicfg.Config, skillName string, refresh bool) error {
	load, err := loadOrRefreshCatalog(cmd.Context(), cfg, catalogOpts{
		forceRefresh:       refresh,
		requireUsableCache: false,
	})
	if err != nil {
		return err
	}
	load.PrintWarn(cmd.ErrOrStderr())

	rows := filterInstalled(BuildInventory(cfg.Aura.Fs(), skillName, cfg.Version, load.Cat))
	renderCheckResult(cmd, cfg, rows)

	drift := countCheckDrift(rows)
	if drift > 0 {
		return clierr.NewValidationError("skill: drift detected in %d skill(s) — run `%s skill install` to refresh", drift, skillName)
	}
	return nil
}

// filterInstalled keeps only rows where the skill is installed for that
// agent. Check is a drift gate — uninstalled rows have nothing to compare.
func filterInstalled(rows []InventoryRow) []InventoryRow {
	out := make([]InventoryRow, 0, len(rows))
	for _, r := range rows {
		if r.Installed {
			out = append(out, r)
		}
	}
	return out
}

// countCheckDrift returns the number of installed rows that should trigger
// a non-zero exit — drift + unknown-version. `installed` (status meaning
// "installed and version-matched") collapses to `ok` for the check view
// (see renderCheckResult).
func countCheckDrift(rows []InventoryRow) int {
	n := 0
	for _, r := range rows {
		if r.Status == statusDrift || r.Status == statusUnknownVersion {
			n++
		}
	}
	return n
}

// checkResultRow is the JSON shape emitted by check. Note `current_version`
// is the same value as list's `available_version` — the column name
// differs by convention (check phrases it as "what the source has right
// now", list phrases it as "what's installable").
type checkResultRow struct {
	Skill            string `json:"skill"`
	Agent            string `json:"agent"`
	InstalledVersion string `json:"installed_version"`
	CurrentVersion   string `json:"current_version"`
	Status           string `json:"status"`
}

// checkResults implements common/output.ResponseData for check results.
type checkResults []checkResultRow

// AsArray returns each row as a column-keyed map for table rendering.
func (r checkResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"skill":             row.Skill,
			"agent":             row.Agent,
			"installed_version": row.InstalledVersion,
			"current_version":   row.CurrentVersion,
			"status":            row.Status,
		})
	}
	return out
}

// MarshalJSON delegates to default slice marshalling, preserving the
// existing JSON array-of-objects shape.
func (r checkResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]checkResultRow(r))
}

func renderCheckResult(cmd *cobra.Command, cfg *clicfg.Config, rows []InventoryRow) {
	out := make(checkResults, 0, len(rows))
	for _, r := range rows {
		// Check sees only installed rows: statusFor returned `installed`
		// when version matched, but check phrases that case as `ok`.
		status := r.Status
		if status == statusInstalled {
			status = statusOk
		}
		out = append(out, checkResultRow{
			Skill:            r.Skill,
			Agent:            r.Agent.Name,
			InstalledVersion: r.InstalledVersion,
			CurrentVersion:   r.AvailableVersion,
			Status:           status,
		})
	}

	if len(out) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Println("No installed skills found.")
		return
	}

	commonoutput.PrintBodyMap(cmd, cfg, out, []string{"skill", "agent", "installed_version", "current_version", "status"})
}
