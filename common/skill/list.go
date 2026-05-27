// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill/catalog"
)

const (
	sourceEmbedded = "embedded"
	sourceCatalog  = "catalog"

	statusInstalled      = "installed"
	statusNotInstalled   = "not-installed"
	statusDrift          = "drift"
	statusUnknownVersion = "unknown-version"
)

func newListCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List skills × agents and per-row install state",
		Long: "Lists one row per (skill × agent) combining the embedded " +
			"self-skill (always first) with every curated catalog skill " +
			"from the cached plugin.json. Columns: skill, source, agent, " +
			"detected, installed, installed_version, available_version, " +
			"status. Auto-refreshes the catalog cache on 24h staleness when " +
			"network is available; otherwise shows cached content. On a " +
			"cold cache only self-skill rows are listed and a hint is " +
			"printed to stderr pointing at `skill refresh`. Use --refresh " +
			"to force a network fetch.",
		Example: `# List skills × agents (table)
neo4j-cli skill list

# List as JSON (machine-readable)
neo4j-cli skill list --format json

# List in toon format
neo4j-cli skill list --format toon

# Force a catalog refresh before listing
neo4j-cli skill list --refresh`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runList(cmd, cfg, skillName, refresh)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force a network refresh of the catalog before listing.")
	return cmd
}

// runList loads the catalog (auto-refresh + soft fallback) and renders
// one row per (skill × agent). On a cold cache it lists only self-skill
// rows and prints a refresh hint to stderr (REQ-F-019).
func runList(cmd *cobra.Command, cfg *clicfg.Config, skillName string, refresh bool) error {
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

	rows := buildListRows(cfg.Aura.Fs(), skillName, cfg.Version, load.Cat)

	if load.Cat == nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skill catalog cache is empty; only self-skill rows shown. Run '%s skill refresh' to populate the catalog.\n", skillName)
	}

	renderListResult(cmd, cfg, rows)
	return nil
}

// ListRow is the per-(skill × agent) view surfaced by the list leaf.
type ListRow struct {
	Skill            string
	Source           string
	Agent            *Agent
	Detected         bool
	Installed        bool
	InstalledVersion string
	AvailableVersion string
	Status           string
}

// buildListRows enumerates one row per (skill × agent). Self-skill rows
// always come first; catalog rows follow in plugin.json order. Reserved
// catalog entries (collisions with self / binary-name) are skipped.
func buildListRows(filesystem afero.Fs, binaryName, binaryVersion string, cat *catalog.Catalog) []ListRow {
	out := make([]ListRow, 0, len(AGENTS))
	out = append(out, rowsForSkill(filesystem, binaryName, sourceEmbedded, binaryVersion)...)
	if cat == nil {
		return out
	}
	for _, entry := range cat.Skills {
		if catalog.IsReserved(entry.Name, binaryName) {
			continue
		}
		out = append(out, rowsForSkill(filesystem, entry.Name, sourceCatalog, cat.Version)...)
	}
	return out
}

// rowsForSkill returns one row per agent for `skillName`. detected/
// installed/installed_version are read from `filesystem`; status is
// computed against `availableVersion`.
func rowsForSkill(filesystem afero.Fs, skillName, source, availableVersion string) []ListRow {
	rows := make([]ListRow, 0, len(AGENTS))
	for i := range AGENTS {
		row := ListRow{
			Skill:            skillName,
			Source:           source,
			Agent:            &AGENTS[i],
			AvailableVersion: availableVersion,
		}
		if dp, ok := AGENTS[i].DetectPath(); ok {
			exists, _ := afero.DirExists(filesystem, dp)
			row.Detected = exists
		}
		row.Installed, row.InstalledVersion = readInstalledSkill(filesystem, &AGENTS[i], skillName)
		row.Status = statusFor(row.Installed, row.InstalledVersion, row.AvailableVersion)
		rows = append(rows, row)
	}
	return rows
}

// statusFor classifies a row. `not-installed` wins over version checks
// because an absent install has no meaningful version to compare.
// `unknown-version` fires when the installed SKILL.md frontmatter lacks a
// parseable `version:` line — distinct from drift so users can see the
// difference between "wrong version" and "no version at all".
func statusFor(installed bool, installedVersion, availableVersion string) string {
	if !installed {
		return statusNotInstalled
	}
	if installedVersion == "" {
		return statusUnknownVersion
	}
	if installedVersion != availableVersion {
		return statusDrift
	}
	return statusInstalled
}

// listResultRow is the JSON shape emitted by list.
type listResultRow struct {
	Skill            string `json:"skill"`
	Source           string `json:"source"`
	Agent            string `json:"agent"`
	Detected         bool   `json:"detected"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	Status           string `json:"status"`
}

// listResults implements common/output.ResponseData for list results.
type listResults []listResultRow

// AsArray returns each row as a column-keyed map for table rendering.
func (r listResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"skill":             row.Skill,
			"source":            row.Source,
			"agent":             row.Agent,
			"detected":          boolStr(row.Detected),
			"installed":         boolStr(row.Installed),
			"installed_version": row.InstalledVersion,
			"available_version": row.AvailableVersion,
			"status":            row.Status,
		})
	}
	return out
}

// MarshalJSON delegates to default slice marshalling, preserving the
// existing JSON array-of-objects shape.
func (r listResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]listResultRow(r))
}

func renderListResult(cmd *cobra.Command, cfg *clicfg.Config, rows []ListRow) {
	out := make(listResults, 0, len(rows))
	for _, r := range rows {
		out = append(out, listResultRow{
			Skill:            r.Skill,
			Source:           r.Source,
			Agent:            r.Agent.Name,
			Detected:         r.Detected,
			Installed:        r.Installed,
			InstalledVersion: r.InstalledVersion,
			AvailableVersion: r.AvailableVersion,
			Status:           r.Status,
		})
	}

	commonoutput.PrintBodyMap(cmd, cfg, out, []string{
		"skill", "source", "agent", "detected", "installed",
		"installed_version", "available_version", "status",
	})
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
