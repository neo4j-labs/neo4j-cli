// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd

import (
	"fmt"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/desktopclient"
	commonoutput "github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// newListCmd generates the `<resource> list` leaf: connect, fetch the catalog,
// render. Table mode goes through the custom renderer (header + one row per
// entry, `(none)` placeholder when empty); json/toon emit the RowJSON-projected
// array (`[]`, never `null`, when empty).
func newListCmd[T any, S any](cfg *clicfg.Config, spec Spec[T, S]) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List " + spec.Plural + " in the local Neo4j Desktop 2 install",
		Long:    spec.ListLong,
		Example: spec.ListExample,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctx := cmd.Context()
			fs := cfg.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			client, err := desktopclient.Connect(ctx, fs, port)
			if err != nil {
				return err
			}

			items, err := spec.List(ctx, client)
			if err != nil {
				return err
			}

			if commonoutput.ResolveOutput(cmd, cfg) == "table" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderTable(spec.Fields, items, spec.RowValues))
				return nil
			}
			commonoutput.PrintBodyMap(cmd, cfg, listResult[T]{items: items, rowJSON: spec.RowJSON}, nil)
			return nil
		},
	}
}
