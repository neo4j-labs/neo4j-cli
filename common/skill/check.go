// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
)

func newCheckCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check installed skills for version drift against this binary",
		Long: "Reads each installed SKILL.md frontmatter `version:` and " +
			"compares to the running binary version. Exits non-zero on any " +
			"drift; prints a per-agent table.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rows, drift, err := Check(cfg.Aura.Fs(), skillName, cfg.Version)
			if err != nil {
				return err
			}
			renderCheckResult(cmd, cfg, rows)
			if drift {
				return fmt.Errorf("skill: drift detected — run `skill install` to refresh")
			}
			return nil
		},
	}
}

// checkResultRow is the JSON shape emitted by check.
type checkResultRow struct {
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

func renderCheckResult(cmd *cobra.Command, cfg *clicfg.Config, rows []CheckRow) {
	out := make(checkResults, 0, len(rows))
	for _, r := range rows {
		out = append(out, checkResultRow{
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

	commonoutput.PrintBodyMap(cmd, cfg, out, []string{"agent", "installed_version", "current_version", "status"})
}
