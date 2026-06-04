// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop

import (
	"encoding/json"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/connection"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// listResult is the unified payload covering Local DBMSes and saved Remote
// connections.
type listResult struct {
	Dbmss       []desktopclient.DbmsInfo   `json:"dbmss"`
	Connections []desktopclient.Connection `json:"connections"`
}

// AsArray satisfies commonoutput.ResponseData.
func (r listResult) AsArray() []map[string]any { return nil }

// MarshalJSON emits `{dbmss, connections}` with both slices non-nil so JSON
// consumers always see an array (possibly empty) under each key, never `null`.
func (r listResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Dbmss       []desktopclient.DbmsInfoOutput   `json:"dbmss"`
		Connections []desktopclient.ConnectionOutput `json:"connections"`
	}{
		Dbmss:       desktopclient.DbmsInfoOutputs(r.Dbmss),
		Connections: desktopclient.ConnectionOutputs(r.Connections),
	})
}

func newListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List local DBMSes and saved remote connections managed by Neo4j Desktop 2",
		Long: "List local DBMSes and saved remote connections managed by the local Neo4j Desktop 2 install — composed view. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"For single-resource views use `neo4j-cli desktop dbms list` (DBMSes only) or `neo4j-cli desktop connection list` (connections only). " +
			"Table format renders two labelled sections: `Local DBMSes` (id, name, version, status, connection_uri) and `Remote connections` (id, name, connection_uri). " +
			"`--format json` emits `{\"dbmss\": [...], \"connections\": [...]}` carrying the full wire payload for each. " +
			"`--format toon` mirrors the JSON shape.",
		Example: `# List DBMSes and saved remote connections as a two-section table
neo4j-cli desktop list

# List as JSON (full payload, agent-friendly) — shape: {dbmss, connections}
neo4j-cli desktop list --format json

# List against a pinned port instead of probing 44222..44232
neo4j-cli desktop list --port 44225`,
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

			// Fan out the two list calls in parallel so total wall time is
			// bounded by the slower endpoint. Either failure aborts the leaf.
			var (
				dbmss       []desktopclient.DbmsInfo
				connections []desktopclient.Connection
			)
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				items, err := client.ListDbmss(gctx)
				if err != nil {
					return err
				}
				dbmss = dbms.EnrichDbmsList(gctx, client, items, cmd.ErrOrStderr())
				return nil
			})
			g.Go(func() error {
				items, err := client.ListConnections(gctx)
				if err != nil {
					return err
				}
				connections = items
				return nil
			})
			if err := g.Wait(); err != nil {
				return err
			}

			result := listResult{Dbmss: dbmss, Connections: connections}
			renderList(cmd, cfg, result)
			return nil
		},
	}
}

func renderList(cmd *cobra.Command, cfg *clicfg.Config, r listResult) {
	if commonoutput.ResolveOutput(cmd, cfg) == "table" {
		printListTable(cmd, r)
		return
	}
	commonoutput.PrintBodyMap(cmd, cfg, r, nil)
}

func printListTable(cmd *cobra.Command, r listResult) {
	cmd.Println("## Local DBMSes")
	cmd.Println(dbms.RenderDbmsTable(r.Dbmss))
	cmd.Println()
	cmd.Println("## Remote connections")
	cmd.Println(connection.RenderConnectionsTable(r.Connections))
}
