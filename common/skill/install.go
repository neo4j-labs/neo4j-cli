// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"io/fs"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

func newInstallCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:   "install [agent]",
		Short: "Install the skill bundle into supported AI agents",
		Long: "Without an argument, installs into every detected agent. " +
			"With an [agent] argument (case-insensitive), installs into that " +
			"single agent. Unknown agent names exit non-zero with the list " +
			"of valid names.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			targets, err := Install(cfg.Aura.Fs(), bundle, skillName, cfg.Version, filter)
			if err != nil {
				return formatAgentErr(err)
			}
			renderInstallResult(cmd, cfg, skillName, "installed", targets)
			return nil
		},
	}
}

// installResultRow is the JSON shape emitted by install/remove.
type installResultRow struct {
	Agent       string `json:"agent"`
	DisplayName string `json:"display_name"`
	SkillsPath  string `json:"skills_path"`
	Action      string `json:"action"`
}

// renderInstallResult prints the install/remove outcome as a table or
// JSON. `action` is "installed" or "removed" — printed in the Action
// column / JSON field. Empty target list emits a friendly note in table
// mode and an empty array in JSON mode.
func renderInstallResult(cmd *cobra.Command, cfg *clicfg.Config, skillName, action string, targets []*Agent) {
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

	if cfg.Aura.Output() == "json" {
		printJSON(cmd, rows)
		return
	}

	if len(rows) == 0 {
		cmd.Printf("No agents to %s.\n", strings.TrimSuffix(action, "ed"))
		return
	}

	t := table.NewWriter()
	t.AppendHeader(table.Row{"agent", "display", "path", "action"})
	for _, r := range rows {
		t.AppendRow(table.Row{r.Agent, r.DisplayName, r.SkillsPath, r.Action})
	}
	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}
