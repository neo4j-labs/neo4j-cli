// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphanalytics

import (
	"github.com/neo4j/cli/common/clicfg"
	sessions "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/graphanalytics/session"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "graph-analytics",
		Short: "Relates to Aura Graph Analytics",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))

			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

			return nil
		},
	}

	cmd.AddCommand(sessions.NewCmd(cfg))

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	return cmd
}
