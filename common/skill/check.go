// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
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

func renderCheckResult(cmd *cobra.Command, cfg *clicfg.Config, rows []CheckRow) {
	out := make([]checkResultRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, checkResultRow{
			Agent:            r.Agent.Name,
			InstalledVersion: r.InstalledVersion,
			CurrentVersion:   r.CurrentVersion,
			Status:           r.Status,
		})
	}

	if cfg.Aura.Output() == "json" {
		printJSON(cmd, out)
		return
	}

	if len(out) == 0 {
		cmd.Println("No installed skills found.")
		return
	}

	t := table.NewWriter()
	t.AppendHeader(table.Row{"agent", "installed-version", "current-version", "status"})
	for _, r := range out {
		t.AppendRow(table.Row{r.Agent, r.InstalledVersion, r.CurrentVersion, r.Status})
	}
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}
