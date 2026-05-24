// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
)

func newListCmd(cfg *clicfg.Config, skillName string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List supported agents and per-agent install state",
		Example: `# List supported agents as a table
neo4j-cli skill list

# List supported agents as JSON (machine-readable)
neo4j-cli skill list --format json

# List supported agents in toon format
neo4j-cli skill list --format toon`,
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
	Agent            string                   `json:"agent"`
	DisplayName      string                   `json:"display_name"`
	Detected         bool                     `json:"detected"`
	Installed        bool                     `json:"installed"`
	InstalledVersion string                   `json:"installed_version"`
	InstallDetails   []installDetailResultRow `json:"install_details,omitempty"`
}

type installDetailResultRow struct {
	Agent            string `json:"agent"`
	DisplayName      string `json:"display_name"`
	SkillsPath       string `json:"skills_path"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
}

// listResults implements common/output.ResponseData for list results.
type listResults []listResultRow

// AsArray returns each row as a column-keyed map for table rendering.
func (r listResults) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, row := range r {
		out = append(out, map[string]any{
			"agent":             row.Agent,
			"display_name":      row.DisplayName,
			"detected":          boolStr(row.Detected),
			"installed":         boolStr(row.Installed),
			"installed_version": row.InstalledVersion,
			"install_details":   formatInstallDetails(row.InstallDetails),
		})
	}
	return out
}

// MarshalJSON delegates to default slice marshalling, preserving the
// existing JSON array-of-objects shape.
func (r listResults) MarshalJSON() ([]byte, error) {
	return json.Marshal([]listResultRow(r))
}

func renderListResult(cmd *cobra.Command, cfg *clicfg.Config, rows []AgentInstall) {
	out := make(listResults, 0, len(rows))
	for _, r := range rows {
		details := make([]installDetailResultRow, 0, len(r.Details))
		if len(r.Details) > 1 {
			for _, detail := range r.Details {
				details = append(details, installDetailResultRow{
					Agent:            detail.Agent,
					DisplayName:      detail.DisplayName,
					SkillsPath:       detail.SkillsPath,
					Installed:        detail.Installed,
					InstalledVersion: detail.InstalledVersion,
				})
			}
		}
		out = append(out, listResultRow{
			Agent:            r.Agent.Name,
			DisplayName:      r.Agent.DisplayName,
			Detected:         r.Detected,
			Installed:        r.Installed,
			InstalledVersion: r.InstalledVersion,
			InstallDetails:   details,
		})
	}

	commonoutput.PrintBodyMap(cmd, cfg, out, []string{"agent", "display_name", "detected", "installed", "installed_version", "install_details"})
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func formatInstallDetails(details []installDetailResultRow) string {
	if len(details) == 0 {
		return ""
	}
	parts := make([]string, 0, len(details))
	for _, detail := range details {
		version := "missing"
		if detail.Installed {
			version = detail.InstalledVersion
			if version == "" {
				version = "unknown-version"
			}
		}
		parts = append(parts, detail.Agent+":"+version)
	}
	return strings.Join(parts, ",")
}
