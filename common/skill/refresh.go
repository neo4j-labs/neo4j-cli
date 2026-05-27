// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/common/skill/catalog"
)

func newRefreshCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "refresh",
		Short:       "Force a fresh download of the curated skill catalog",
		Annotations: map[string]string{"write": "true"},
		Long: "Forces a network fetch of the curated catalog `plugin.json` " +
			"from github.com/neo4j-contrib/neo4j-skills. When the upstream " +
			"version differs from the cached one, the repo tarball is re-" +
			"downloaded and extracted into the local cache. On network " +
			"failure with a usable cache the previous content is preserved " +
			"and a warning is emitted to stderr; on network failure with no " +
			"cache the command exits non-zero with a connectivity hint.",
		Example: `# Force a catalog refresh
neo4j-cli skill refresh --rw

# Emit the result as JSON (machine-readable)
neo4j-cli skill refresh --format json --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return runRefresh(cmd, cfg, skillName)
		},
	}
	return cmd
}

// runRefresh always forces a catalog refresh and renders the resulting
// version + addressable skill count. Cold cache + network failure is
// fatal with a connectivity hint (REQ-F-018); warm cache + network
// failure logs a warning to stderr and reports the cached state
// (REQ-F-019).
func runRefresh(cmd *cobra.Command, cfg *clicfg.Config, skillName string) error {
	load, err := loadOrRefreshCatalog(cmd.Context(), cfg, catalogOpts{
		forceRefresh:       true,
		requireUsableCache: false,
	})
	if err != nil {
		return err
	}

	if load.Cat == nil {
		return clierr.NewUsageError(
			"skill: catalog refresh failed and no local cache available: %v\ncheck your network connectivity and try again",
			load.Warn,
		)
	}

	if load.Warn != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: skill catalog refresh failed, using cached content: %v\n", load.Warn)
	}

	row := refreshResultRow{
		Version:    load.Cat.Version,
		SkillCount: countAddressableSkills(load.Cat.Skills, skillName),
		Source:     sourceCatalog,
	}
	renderRefreshResult(cmd, cfg, row)
	return nil
}

// countAddressableSkills counts catalog entries that are not reserved
// (i.e. don't collide with the self-skill identity) — matches the count
// of skills a user can actually install/print by name.
func countAddressableSkills(skills []catalog.SkillEntry, binaryName string) int {
	n := 0
	for _, s := range skills {
		if catalog.IsReserved(s.Name, binaryName) {
			continue
		}
		n++
	}
	return n
}

// refreshResultRow is the JSON shape emitted by refresh.
type refreshResultRow struct {
	Version    string `json:"version"`
	SkillCount int    `json:"skill_count"`
	Source     string `json:"source"`
}

// refreshResults implements common/output.ResponseData for the single-row
// refresh output.
type refreshResults struct {
	Row refreshResultRow
}

// AsArray returns the single row as a column-keyed map for table render.
func (r refreshResults) AsArray() []map[string]any {
	return []map[string]any{{
		"version":     r.Row.Version,
		"skill_count": r.Row.SkillCount,
		"source":      r.Row.Source,
	}}
}

// MarshalJSON emits the single row as an object (not an array) so machine
// consumers can read `.version` directly without indexing.
func (r refreshResults) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Row)
}

func renderRefreshResult(cmd *cobra.Command, cfg *clicfg.Config, row refreshResultRow) {
	commonoutput.PrintBodyMap(cmd, cfg, refreshResults{Row: row}, []string{"version", "skill_count", "source"})
}
