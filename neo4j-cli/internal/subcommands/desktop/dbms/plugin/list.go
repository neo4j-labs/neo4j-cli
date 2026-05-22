// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package plugin

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

const portFlag = "port"

// JSON/toon output bypasses this list so consumers see every wire field.
var pluginListFields = []string{"name", "version", "pendingRestart", "filePath"}

// pluginListResult is the payload returned by `plugin list` and `plugin available`.
type pluginListResult struct {
	Plugins []desktopclient.DbmsPlugin `json:"plugins"`
}

func (r pluginListResult) AsArray() []map[string]any { return nil }

// MarshalJSON emits the DbmsPlugin array directly; empty case marshals to `[]` (never `null`).
func (r pluginListResult) MarshalJSON() ([]byte, error) {
	plugins := r.Plugins
	if plugins == nil {
		plugins = []desktopclient.DbmsPlugin{}
	}
	return json.Marshal(plugins)
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list <dbms-id>",
		Short: "List plugins installed on a local Desktop-managed DBMS",
		Long: "List plugins installed on a local Neo4j Desktop 2-managed DBMS. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"`<dbms-id>` is the DBMS id (Desktop UUID); see `neo4j-cli desktop dbms list` for the catalog. " +
			"`--format json` emits a JSON array of full `DbmsPlugin` objects (every wire field Desktop returns). " +
			"`--format toon` mirrors the JSON shape. " +
			"A `pendingRestart: true` entry means the plugin JAR is on disk but the running DBMS has not yet been restarted to pick it up — restart the DBMS or pass `--no-restart` to `install`/`uninstall` to defer the restart explicitly.",
		Example: `# List installed plugins on a DBMS as a table
neo4j-cli desktop dbms plugin list my-dbms-id

# List installed plugins as JSON (full DbmsPlugin payload, agent-friendly)
neo4j-cli desktop dbms plugin list my-dbms-id --format json

# List installed plugins against a pinned port instead of probing 44222..44232
neo4j-cli desktop dbms plugin list my-dbms-id --port 44225`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)
			dbmsID := args[0]

			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}

			plugins, err := client.ListInstalledPlugins(ctx, dbmsID)
			if err != nil {
				if errors.Is(err, desktopclient.ErrDbmsNotFound) {
					return dbmsNotFoundError(dbmsID)
				}
				return err
			}

			renderPluginList(cmd, cfg, pluginListResult{Plugins: plugins})
			return nil
		},
	}
}

// renderPluginList dispatches on the resolved output format.
func renderPluginList(cmd *cobra.Command, cfg *clicfg.Config, r pluginListResult) {
	if commonoutput.ResolveOutput(cmd, cfg) == "table" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderPluginTable(r.Plugins))
		return
	}
	commonoutput.PrintBodyMap(cmd, cfg, r, nil)
}

// renderPluginTable emits the plugin table; empty input yields a `(none)` placeholder row.
func renderPluginTable(items []desktopclient.DbmsPlugin) string {
	t := table.NewWriter()
	header := make(table.Row, 0, len(pluginListFields))
	for _, f := range pluginListFields {
		header = append(header, f)
	}
	t.AppendHeader(header)
	if len(items) == 0 {
		row := make(table.Row, len(pluginListFields))
		row[0] = "(none)"
		for i := 1; i < len(row); i++ {
			row[i] = ""
		}
		t.AppendRow(row)
	} else {
		for _, p := range items {
			t.AppendRow(table.Row{
				commonoutput.StripControl(p.Name),
				commonoutput.StripControl(p.Version),
				p.PendingRestart,
				commonoutput.StripControl(p.FilePath),
			})
		}
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}
