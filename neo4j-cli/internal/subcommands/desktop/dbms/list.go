// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"encoding/json"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// dbmsListResult is the DBMS-only payload returned by `desktop dbms list`.
// The composed `{dbmss, connections}` envelope lives on `desktop list`.
type dbmsListResult struct {
	Dbmss []desktopclient.DbmsInfo `json:"dbmss"`
}

func (r dbmsListResult) AsArray() []map[string]any { return nil }

// MarshalJSON emits the array directly (not wrapped) so `--format json` returns
// `[{...}, {...}]`. Empty case marshals to `[]` (never `null`).
func (r dbmsListResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(desktopclient.DbmsInfoOutputs(r.Dbmss))
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local DBMSes managed by Neo4j Desktop 2",
		Long: "List local DBMSes managed by the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Returns DBMSes only; for the composed view that also includes saved remote connections see `neo4j-cli desktop list`. " +
			"For saved remote connections alone see `neo4j-cli desktop connection list`. " +
			"`--format json` emits a JSON array of full `DbmsInfo` objects (every wire field Desktop returns). " +
			"`--format toon` mirrors the JSON shape.",
		Example: `# List local DBMSes as a table
neo4j-cli desktop dbms list

# List local DBMSes as JSON (full DbmsInfo payload, agent-friendly)
neo4j-cli desktop dbms list --format json

# List local DBMSes against a pinned port instead of probing 44222..44232
neo4j-cli desktop dbms list --port 44225`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}

			items, err := client.ListDbmss(ctx)
			if err != nil {
				return err
			}
			// Desktop's `GET /dbmss` returns a slim shape; per-entry GET fills
			// in status / version / edition for the table.
			dbmss := EnrichDbmsList(ctx, client, items, cmd.ErrOrStderr())

			result := dbmsListResult{Dbmss: dbmss}
			renderDbmsList(cmd, cfg, result)
			return nil
		},
	}
}

// renderDbmsList dispatches on the resolved output format.
func renderDbmsList(cmd *cobra.Command, cfg *clicfg.Config, r dbmsListResult) {
	if commonoutput.ResolveOutput(cmd, cfg) == "table" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), RenderDbmsTable(r.Dbmss))
		return
	}
	commonoutput.PrintBodyMap(cmd, cfg, r, nil)
}

// RenderDbmsTable emits the Local DBMSes table; empty input yields a one-row
// `(none)` placeholder so the table is visually present. Reused by the composed
// `desktop list` view.
func RenderDbmsTable(items []desktopclient.DbmsInfo) string {
	t := table.NewWriter()
	header := make(table.Row, 0, len(DbmsListFields))
	for _, f := range DbmsListFields {
		header = append(header, f)
	}
	t.AppendHeader(header)
	if len(items) == 0 {
		row := make(table.Row, len(DbmsListFields))
		row[0] = "(none)"
		for i := 1; i < len(row); i++ {
			row[i] = ""
		}
		t.AppendRow(row)
	} else {
		for _, info := range items {
			t.AppendRow(table.Row{
				commonoutput.StripControl(info.ID),
				commonoutput.StripControl(info.Name),
				commonoutput.StripControl(info.Version),
				commonoutput.StripControl(info.Status),
				commonoutput.StripControl(info.ConnectionURI),
			})
		}
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}
