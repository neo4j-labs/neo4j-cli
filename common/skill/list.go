// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
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
// the full inventory. On a cold cache it lists only self-skill rows and
// prints a refresh hint to stderr (REQ-F-019).
func runList(cmd *cobra.Command, cfg *clicfg.Config, skillName string, refresh bool) error {
	load, err := loadOrRefreshCatalog(cmd.Context(), cfg, catalogOpts{
		forceRefresh:       refresh,
		requireUsableCache: false,
	})
	if err != nil {
		return err
	}
	load.PrintWarn(cmd.ErrOrStderr())

	rows := BuildInventory(cfg.Aura.Fs(), skillName, cfg.Version, load.Cat)

	load.PrintColdCacheHint(cmd.ErrOrStderr(), skillName)

	renderListResult(cmd, cfg, rows)
	return nil
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
// JSON/toon paths bypass this via MarshalJSON to preserve real booleans;
// the table renderer needs yes/no strings for readability.
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
// existing JSON array-of-objects shape with real booleans.
func (r listResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]listResultRow(r))
}

func renderListResult(cmd *cobra.Command, cfg *clicfg.Config, rows []InventoryRow) {
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
