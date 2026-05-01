// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tenant

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Relates to an Aura Tenant",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))

			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

			return nil
		},
	}

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	cmd.AddCommand(NewGetCmd(cfg))
	cmd.AddCommand(NewListCmd(cfg))

	return cmd
}
