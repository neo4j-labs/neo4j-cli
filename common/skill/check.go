// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill/catalog"
)

func newCheckCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check installed skills for version drift",
		Long: "Inspects every installed skill across detected agents and " +
			"compares its frontmatter `version:` against the source " +
			"version (binary version for the self-skill, plugin.json " +
			"version for catalog skills). Columns: skill, agent, " +
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
// installed-only check rows, renders them, and exits non-zero when any
// row reports drift or unknown-version.
func runCheck(cmd *cobra.Command, cfg *clicfg.Config, skillName string, refresh bool) error {
	load, err := loadOrRefreshCatalog(cmd.Context(), cfg, catalogOpts{
		forceRefresh:       refresh,
		requireUsableCache: false,
	})
	if err != nil {
		return err
	}
	if load.Warn != nil && load.Cat != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skill catalog refresh failed, using cached content: %v\n", load.Warn)
	}

	rows := buildCheckRows(cfg.Aura.Fs(), skillName, cfg.Version, load.Cat)
	renderCheckResult(cmd, cfg, rows)

	drift := countDrift(rows)
	if drift > 0 {
		return clierr.NewValidationError("skill: drift detected in %d skill(s) — run `%s skill install` to refresh", drift, skillName)
	}
	return nil
}

// CheckRowView is the per-(installed skill × agent) view surfaced by the
// check leaf. Distinct from the legacy CheckRow returned by Check() so the
// older self-only API stays binary-compatible for its existing callers.
type CheckRowView struct {
	Skill            string
	Agent            *Agent
	InstalledVersion string
	CurrentVersion   string
	Status           string
}

// buildCheckRows iterates self + catalog skills across the AGENTS catalog
// and emits one row per *installed* (skill × agent). Reserved catalog
// entries (collisions with self / binary-name) are skipped.
func buildCheckRows(filesystem afero.Fs, binaryName, binaryVersion string, cat *catalog.Catalog) []CheckRowView {
	out := make([]CheckRowView, 0, len(AGENTS))
	out = append(out, checkRowsForSkill(filesystem, binaryName, binaryVersion)...)
	if cat == nil {
		return out
	}
	for _, entry := range cat.Skills {
		if catalog.IsReserved(entry.Name, binaryName) {
			continue
		}
		out = append(out, checkRowsForSkill(filesystem, entry.Name, cat.Version)...)
	}
	return out
}

// checkRowsForSkill returns one row per agent where `skillName` is
// installed. Uninstalled agents are silently omitted — check is a drift
// gate, not a presence report.
func checkRowsForSkill(filesystem afero.Fs, skillName, currentVersion string) []CheckRowView {
	rows := make([]CheckRowView, 0, len(AGENTS))
	for i := range AGENTS {
		installed, installedVersion := readInstalledSkill(filesystem, &AGENTS[i], skillName)
		if !installed {
			continue
		}
		rows = append(rows, CheckRowView{
			Skill:            skillName,
			Agent:            &AGENTS[i],
			InstalledVersion: installedVersion,
			CurrentVersion:   currentVersion,
			Status:           checkStatus(installedVersion, currentVersion),
		})
	}
	return rows
}

// checkStatus classifies one (installed) row. `unknown-version` fires
// when the frontmatter has no parseable `version:` line; `drift` fires
// when the parsed version disagrees with the source version; otherwise
// the row is `ok`.
func checkStatus(installedVersion, currentVersion string) string {
	if installedVersion == "" {
		return statusUnknownVersion
	}
	if installedVersion != currentVersion {
		return statusDrift
	}
	return checkStatusOk
}

const checkStatusOk = "ok"

// countDrift returns the count of rows that should trigger a non-zero
// exit — drift + unknown-version. Used by runCheck to decide whether to
// return a ValidationError.
func countDrift(rows []CheckRowView) int {
	n := 0
	for _, r := range rows {
		if r.Status == statusDrift || r.Status == statusUnknownVersion {
			n++
		}
	}
	return n
}

// checkResultRow is the JSON shape emitted by check.
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

func renderCheckResult(cmd *cobra.Command, cfg *clicfg.Config, rows []CheckRowView) {
	out := make(checkResults, 0, len(rows))
	for _, r := range rows {
		out = append(out, checkResultRow{
			Skill:            r.Skill,
			Agent:            r.Agent.Name,
			InstalledVersion: r.InstalledVersion,
			CurrentVersion:   r.CurrentVersion,
			Status:           r.Status,
		})
	}

	if len(out) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Println("No installed skills found.")
		return
	}

	commonoutput.PrintBodyMap(cmd, cfg, out, []string{"skill", "agent", "installed_version", "current_version", "status"})
}
