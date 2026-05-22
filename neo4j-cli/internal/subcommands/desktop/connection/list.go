// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection

import (
	"encoding/json"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// ConnectionListFields is the default column order for the Remote connections
// table; JSON / toon emit the full Connection array.
var ConnectionListFields = []string{"id", "name", "connectionUri"}

// connectionListResult is the payload returned by `desktop connection list`.
type connectionListResult struct {
	Connections []desktopclient.Connection `json:"connections"`
}

// AsArray satisfies commonoutput.ResponseData; table mode routes through a
// custom renderer so this returns nil.
func (r connectionListResult) AsArray() []map[string]any { return nil }

// MarshalJSON emits the Connection array unwrapped. Empty marshals to `[]`
// (never `null`) so JSON consumers always see an array.
func (r connectionListResult) MarshalJSON() ([]byte, error) {
	conns := r.Connections
	if conns == nil {
		conns = []desktopclient.Connection{}
	}
	return json.Marshal(conns)
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved remote DB connections registered with Neo4j Desktop 2",
		Long: "List saved remote DB connections registered with the local Neo4j Desktop 2 install. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"Returns connections only; for the composed view that also includes local DBMSes see `neo4j-cli desktop list`. " +
			"For local DBMSes alone see `neo4j-cli desktop dbms list`. " +
			"`--format json` emits a JSON array of full `Connection` objects (every wire field Desktop returns). " +
			"`--format toon` mirrors the JSON shape.",
		Example: `# List saved remote connections as a table
neo4j-cli desktop connection list

# List saved remote connections as JSON (full Connection payload, agent-friendly)
neo4j-cli desktop connection list --format json

# List saved remote connections against a pinned port instead of probing 44222..44232
neo4j-cli desktop connection list --port 44225`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt("port")

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}

			items, err := client.ListConnections(ctx)
			if err != nil {
				return err
			}

			result := connectionListResult{Connections: items}
			renderConnectionList(cmd, cfg, result)
			return nil
		},
	}
}

// renderConnectionList dispatches on the resolved output format.
func renderConnectionList(cmd *cobra.Command, cfg *clicfg.Config, r connectionListResult) {
	if commonoutput.ResolveOutput(cmd, cfg) == "table" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), RenderConnectionsTable(r.Connections))
		return
	}
	commonoutput.PrintBodyMap(cmd, cfg, r, nil)
}

// RenderConnectionsTable emits the Remote connections table; exported so the
// composed `desktop list` view reuses the same rendering. Empty input yields a
// one-row `(none)` placeholder so the table is visually present.
func RenderConnectionsTable(items []desktopclient.Connection) string {
	t := table.NewWriter()
	header := make(table.Row, 0, len(ConnectionListFields))
	for _, f := range ConnectionListFields {
		header = append(header, f)
	}
	t.AppendHeader(header)
	if len(items) == 0 {
		row := make(table.Row, len(ConnectionListFields))
		row[0] = "(none)"
		for i := 1; i < len(row); i++ {
			row[i] = ""
		}
		t.AppendRow(row)
	} else {
		for _, c := range items {
			t.AppendRow(table.Row{
				commonoutput.StripControl(c.ID),
				commonoutput.StripControl(c.Name),
				commonoutput.StripControl(c.ConnectionURI),
			})
		}
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}
