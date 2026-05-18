// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package workspace

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage the active organization and project workspace",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))
			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))
			return nil
		},
	}

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newUseCmd(cfg))

	flags.RegisterAuraCredentialFlag(cmd, cfg)

	return cmd
}
