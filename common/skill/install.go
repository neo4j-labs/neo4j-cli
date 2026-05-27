// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
)

func newInstallCmd(cfg *clicfg.Config, bundle fs.FS, skillName string) *cobra.Command {
	var agentFilter string
	cmd := &cobra.Command{
		Use:         "install [skill-name]",
		Short:       "Install a skill bundle into supported AI agents",
		Annotations: map[string]string{"write": "true"},
		Long: "Without a positional, installs the embedded self-skill into " +
			"every detected agent. With a [skill-name] positional, installs " +
			"that named skill (self-skill or catalog skill). Use --agent " +
			"<name> (case-insensitive) to scope the install to one agent. " +
			"Passing an agent name as the positional is a hard error — use " +
			"--agent <name> instead." +
			"\n\nSupported agents: " + strings.Join(agentNames(), ", "),
		Example: `# Install the embedded self-skill into every detected agent
neo4j-cli skill install --rw

# Install the embedded self-skill into a single agent
neo4j-cli skill install --agent claude-code --rw

# Install and emit the result as JSON (machine-readable)
neo4j-cli skill install --format json --rw`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			skillArg := ""
			if len(args) == 1 {
				skillArg = args[0]
			}

			src, err := resolveSkillSource(bundle, cfg.Version, skillName, skillArg)
			if err != nil {
				return err
			}

			// Self-skill installs always land at `<binaryName>/` on disk;
			// catalog skills (task-007) will install at `<catalog-name>/`.
			targets, err := Install(cfg.Aura.Fs(), src, skillName, agentFilter)
			if err != nil {
				return formatAgentErr(err)
			}
			renderInstallResult(cmd, cfg, skillName, "installed", targets)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFilter, "agent", "", "Restrict install to a single agent (case-insensitive). See --help for supported agents.")
	return cmd
}

// resolveSkillSource maps a positional skill-name to the Source the
// installer should copy. Empty arg defaults to the embedded self-skill.
// A named arg resolves via the self-skill resolver first; anything else
// is a hard-break (agent-name collision) or an unknown-skill error.
// Catalog Lookup will plug in here in task-007.
func resolveSkillSource(bundle fs.FS, version, binaryName, skillArg string) (Source, error) {
	if skillArg == "" {
		return Source{FS: bundle, Version: version}, nil
	}
	src, err := ResolveSelf(bundle, version, binaryName, skillArg)
	if err == nil {
		return src, nil
	}
	if !errors.Is(err, ErrNotSelfSkill) {
		return Source{}, err
	}
	if isAgentName(skillArg) {
		return Source{}, didYouMeanAgentErr(skillArg)
	}
	return Source{}, unknownSkillErr(skillArg)
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
