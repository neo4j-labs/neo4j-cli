// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
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

// installResults implements common/output.ResponseData for install/remove results.
type installResults struct {
	rows   []installResultRow
	action string
}

// AsArray returns each row as a column-keyed map for table rendering.
func (r installResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r.rows))
	for _, row := range r.rows {
		out = append(out, map[string]any{
			"agent":        row.Agent,
			"display_name": row.DisplayName,
			"skills_path":  row.SkillsPath,
			"action":       row.Action,
		})
	}
	return out
}

// MarshalJSON delegates to default slice marshalling, preserving the
// existing JSON array-of-objects shape.
func (r installResults) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.rows)
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

	data := installResults{rows: rows, action: action}

	if len(rows) == 0 && commonoutput.ResolveOutput(cmd, cfg) != "json" {
		cmd.Printf("No agents to %s.\n", strings.TrimSuffix(action, "ed"))
		return
	}

	commonoutput.PrintBodyMap(cmd, cfg, data, []string{"agent", "display_name", "skills_path", "action"})
}
