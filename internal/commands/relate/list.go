// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"encoding/json"
	"time"

	engine "github.com/neo4j/cli/internal/relate"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newListCmd builds the `neo4j-cli relate list` leaf: it scans the CLI's
// managed store directory and renders one row per store found. Stores
// created at explicit paths outside the managed directory are not listed
// (the filesystem is the source of truth; there is no registry). Works in
// every build — no engine needed. An empty result renders as an empty table
// or empty JSON array with exit 0.
func newListCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Relate graph stores under the managed directory",
		Long: "List every Relate graph store under the CLI's managed store directory, one row per store: " +
			"name (the bare name `query --relate <name>` resolves), kind (directory or .rl bundle), " +
			"size in bytes, and last modified. Stores created at explicit paths are not listed. " +
			"An empty result renders as an empty table or empty JSON array (exit 0).",
		Example: `# List managed stores as a table
neo4j-cli relate list

# Emit JSON for scripting
neo4j-cli relate list --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			stores, err := engine.ManagedStores(cfg.Fs())
			if err != nil {
				cmd.SilenceUsage = true
				return clierr.NewFatalError("cannot read the managed store directory: %w", err)
			}

			rows := make([]map[string]any, 0, len(stores))
			for _, s := range stores {
				rows = append(rows, map[string]any{
					"name":     s.Name,
					"kind":     s.Kind,
					"size":     s.Size,
					"modified": s.Modified.Format(time.RFC3339),
				})
			}

			fields := []string{"name", "kind", "size", "modified"}
			commonoutput.PrintBodyMap(cmd, cfg, listRows(rows), fields)
			return nil
		},
	}

	return cmd
}

// listRows adapts a []map[string]any into a commonoutput.ResponseData,
// preserving cardinality (empty renders as `[]`, never `null`).
type listRows []map[string]any

// AsArray implements commonoutput.ResponseData.
func (r listRows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

// MarshalJSON returns the JSON array form so the empty case renders as `[]`.
func (r listRows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}
