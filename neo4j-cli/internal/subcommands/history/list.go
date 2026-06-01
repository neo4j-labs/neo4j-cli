// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

const defaultListLimit = 20

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recently run neo4j-cli commands, newest first",
		Long: "List the most recent neo4j-cli commands recorded in the local history log, newest first. " +
			"Shows the last 20 entries by default; override with --limit. " +
			"The default and table views render the human form `[time] <command> {invoker:...}`; " +
			"--format json|toon emits the structured entries.",
		Example: `# Show the last 20 commands, newest first
neo4j-cli history list

# Show the last 5 commands
neo4j-cli history list --limit 5

# Emit the full structured history as JSON
neo4j-cli history list --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := Load(cfg)
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}

			// Newest first.
			for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
				entries[i], entries[j] = entries[j], entries[i]
			}

			if limit > 0 && len(entries) > limit {
				entries = entries[:limit]
			}

			switch commonoutput.ResolveOutput(cmd, cfg) {
			case "json", "toon":
				commonoutput.PrintBodyMap(cmd, cfg, historyRows(entries), nil)
			default:
				for _, e := range entries {
					cmd.Println(humanLine(e))
				}
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", defaultListLimit, "Maximum number of entries to show, newest first (0 = all)")

	return cmd
}

// humanLine renders one entry as `[time] <command> {invoker:..., workspace:..., credential:...}`.
// workspace and credential are included only when present.
func humanLine(e Entry) string {
	var meta strings.Builder
	meta.WriteString("invoker:" + e.Invoker)
	if e.Workspace != "" {
		meta.WriteString(", workspace:" + e.Workspace)
	}
	if e.Credential != "" {
		meta.WriteString(", credential:" + e.Credential)
	}
	return fmt.Sprintf("[%s] %s {%s}", e.Time.Format(time.RFC3339), e.Command, meta.String())
}

// historyRows adapts a slice of entries into commonoutput.ResponseData so the
// json/toon paths render an array (empty → `[]`, not `null`).
type historyRows []Entry

func (r historyRows) AsArray() []map[string]any {
	out := make([]map[string]any, 0, len(r))
	for _, e := range r {
		row := map[string]any{
			"time":    e.Time.Format(time.RFC3339),
			"command": e.Command,
			"invoker": e.Invoker,
			"version": e.Version,
		}
		if e.Workspace != "" {
			row["workspace"] = e.Workspace
		}
		if e.Credential != "" {
			row["credential"] = e.Credential
		}
		out = append(out, row)
	}
	return out
}

func (r historyRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}
