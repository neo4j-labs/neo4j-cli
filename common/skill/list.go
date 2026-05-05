// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

func newListCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List supported agents and per-agent install state",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rows, err := List(cfg.Aura.Fs(), skillName)
			if err != nil {
				return err
			}
			renderListResult(cmd, cfg, rows)
			return nil
		},
	}
}

// listResultRow is the JSON shape emitted by list.
type listResultRow struct {
	Agent            string `json:"agent"`
	DisplayName      string `json:"display_name"`
	Detected         bool   `json:"detected"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
}

func renderListResult(cmd *cobra.Command, cfg *clicfg.Config, rows []AgentInstall) {
	out := make([]listResultRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, listResultRow{
			Agent:            r.Agent.Name,
			DisplayName:      r.Agent.DisplayName,
			Detected:         r.Detected,
			Installed:        r.Installed,
			InstalledVersion: r.InstalledVersion,
		})
	}

	if cfg.Aura.Output() == "json" {
		printJSON(cmd, out)
		return
	}

	t := table.NewWriter()
	t.AppendHeader(table.Row{"agent", "display", "detected", "installed", "installed-version"})
	for _, r := range out {
		t.AppendRow(table.Row{r.Agent, r.DisplayName, boolStr(r.Detected), boolStr(r.Installed), r.InstalledVersion})
	}
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
