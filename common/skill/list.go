// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
)

func newListCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List skills × agents and per-row install state",
		Long: "Lists the embedded self-skill and curated catalog skills " +
			"from the cached plugin.json. Default table/toon output is a " +
			"compact two-section view: an 11-row self-skill matrix " +
			"(columns: agent, detected, installed, installed_version, " +
			"available_version, status) followed by an aggregated catalog " +
			"section (columns: skill, available_version, status, " +
			"installed_in). --format json keeps the flat per-(skill × " +
			"agent) array shape for back-compat with script consumers. " +
			"Auto-refreshes the catalog cache on 24h staleness when " +
			"network is available; otherwise shows cached content. On a " +
			"cold cache only the self-skill section renders and a hint is " +
			"printed to stderr pointing at `skill refresh`. Use --refresh " +
			"to force a network fetch.",
		Example: `# List skills × agents (table)
neo4j-cli skill list

# List as JSON (machine-readable, flat per-(skill × agent) array)
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
// the inventory. JSON output keeps the flat per-(skill × agent) shape;
// table/toon output renders a two-section compact view. On a cold cache
// only the self-skill section renders and a refresh hint goes to stderr.
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

// listResults implements common/output.ResponseData for the flat JSON
// view. Kept as today's shape because --format json is a frozen contract.
type listResults []listResultRow

// AsArray returns each row as a column-keyed map.
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

// selfResultRow is the per-agent shape for the self-skill section
// (table/toon). It drops `skill` and `source` because the section
// heading carries that context.
type selfResultRow struct {
	Agent            string `json:"agent"`
	Detected         bool   `json:"detected"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	Status           string `json:"status"`
}

type selfResults []selfResultRow

func (r selfResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
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

func (r selfResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]selfResultRow(r))
}

// catalogResultRow is the per-skill shape for the catalog section. It
// folds the 11 per-agent rows into a single summary with worst-wins
// status + an installed_in cell naming which agents hold it.
type catalogResultRow struct {
	Skill            string `json:"skill"`
	AvailableVersion string `json:"available_version"`
	Status           string `json:"status"`
	InstalledIn      string `json:"installed_in"`
}

type catalogResults []catalogResultRow

func (r catalogResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"skill":             row.Skill,
			"available_version": row.AvailableVersion,
			"status":            row.Status,
			"installed_in":      row.InstalledIn,
		})
	}
	return out
}

func (r catalogResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]catalogResultRow(r))
}

func renderListResult(cmd *cobra.Command, cfg *clicfg.Config, rows []InventoryRow) {
	// JSON shape is frozen contract — keep flat.
	if commonoutput.ResolveOutput(cmd, cfg) == "json" {
		renderListJSON(cmd, cfg, rows)
		return
	}
	renderListTwoSection(cmd, cfg, rows)
}

func renderListJSON(cmd *cobra.Command, cfg *clicfg.Config, rows []InventoryRow) {
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

func renderListTwoSection(cmd *cobra.Command, cfg *clicfg.Config, rows []InventoryRow) {
	selfRows, catalogGroups := splitInventory(rows)

	cmd.Println("Self-skill:")
	cmd.Println()
	commonoutput.PrintBodyMap(cmd, cfg, selfRows, []string{
		"agent", "detected", "installed",
		"installed_version", "available_version", "status",
	})

	if len(catalogGroups) == 0 {
		return
	}

	cmd.Println()
	cmd.Println("Catalog:")
	cmd.Println()

	catRows := make(catalogResults, 0, len(catalogGroups))
	for _, group := range catalogGroups {
		s := aggregateCatalog(group)
		catRows = append(catRows, catalogResultRow{
			Skill:            s.Skill,
			AvailableVersion: s.AvailableVersion,
			Status:           s.Status,
			InstalledIn:      formatInstalledIn(s),
		})
	}
	commonoutput.PrintBodyMap(cmd, cfg, catRows, []string{
		"skill", "available_version", "status", "installed_in",
	})
}

// splitInventory partitions rows into the self-skill section and an
// ordered list of catalog-skill groups (each group is the per-agent rows
// for one catalog skill, preserved in plugin.json order via the input
// order produced by BuildInventory).
func splitInventory(rows []InventoryRow) (selfResults, [][]InventoryRow) {
	self := make(selfResults, 0, skillAgentCount())
	groups := make([][]InventoryRow, 0)
	index := map[string]int{}
	for _, r := range rows {
		if r.Source == sourceEmbedded {
			self = append(self, selfResultRow{
				Agent:            r.Agent.Name,
				Detected:         r.Detected,
				Installed:        r.Installed,
				InstalledVersion: r.InstalledVersion,
				AvailableVersion: r.AvailableVersion,
				Status:           r.Status,
			})
			continue
		}
		idx, ok := index[r.Skill]
		if !ok {
			idx = len(groups)
			index[r.Skill] = idx
			groups = append(groups, []InventoryRow{})
		}
		groups[idx] = append(groups[idx], r)
	}
	return self, groups
}

// formatInstalledIn renders the catalog row's installed_in cell:
//   - "—" when no agent holds the skill,
//   - "N/M" when every skill-capable agent holds it,
//   - "N/M (a, b, ...)" otherwise.
func formatInstalledIn(s catalogSummary) string {
	total := skillAgentCount()
	if s.InstalledCount == 0 {
		return "—"
	}
	if s.InstalledCount == total {
		return fmt.Sprintf("%d/%d", s.InstalledCount, total)
	}
	return fmt.Sprintf("%d/%d (%s)", s.InstalledCount, total, strings.Join(s.InstalledAgents, ", "))
}
