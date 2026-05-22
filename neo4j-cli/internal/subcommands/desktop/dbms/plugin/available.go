// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package plugin

import (
	"errors"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

func newAvailableCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "available <dbms-id>",
		Short: "List the installable plugin catalog for a local Desktop-managed DBMS",
		Long: "List the installable plugin catalog Neo4j Desktop 2 exposes for a local managed DBMS. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. " +
			"`<dbms-id>` is the DBMS id (Desktop UUID); see `neo4j-cli desktop dbms list` for the catalog. " +
			"`--format json` emits a JSON array of full `DbmsPlugin` objects (every wire field Desktop returns). " +
			"`--format toon` mirrors the JSON shape. " +
			"Pair with `neo4j-cli desktop dbms plugin install <dbms-id> <name>` to install one of the entries.",
		Example: `# Browse the installable plugin catalog for a DBMS as a table
neo4j-cli desktop dbms plugin available my-dbms-id

# List the installable catalog as JSON (full DbmsPlugin payload, agent-friendly)
neo4j-cli desktop dbms plugin available my-dbms-id --format json

# Browse against a pinned port instead of probing 44222..44232
neo4j-cli desktop dbms plugin available my-dbms-id --port 44225`,
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

			plugins, err := client.ListAvailablePlugins(ctx, dbmsID)
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
