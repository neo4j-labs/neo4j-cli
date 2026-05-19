// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agent

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "agent",
		Short: "Relates to Aura Agents",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))
			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

			return nil
		},
	}

	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewGetCmd(cfg))
	cmd.AddCommand(NewCreateCmd(cfg))
	cmd.AddCommand(NewUpdateCmd(cfg))
	cmd.AddCommand(NewReplaceCmd(cfg))
	cmd.AddCommand(NewDeleteCmd(cfg))
	cmd.AddCommand(NewInvokeCmd(cfg))

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	flags.RegisterAuraCredentialFlag(cmd, cfg)

	return cmd
}
