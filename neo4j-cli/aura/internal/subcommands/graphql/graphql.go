// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/graphql/authprovider"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/graphql/corspolicy"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "graphql",
		Short: "Allows you to programmatically provision and manage your GraphQL Data APIs",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))
			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

			return nil
		},
	}

	cmd.AddCommand(authprovider.NewCmd(cfg))
	cmd.AddCommand(corspolicy.NewCmd(cfg))
	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewGetCmd(cfg))
	cmd.AddCommand(NewUpdateCmd(cfg))
	cmd.AddCommand(NewCreateCmd(cfg))
	cmd.AddCommand(NewDeleteCmd(cfg))
	cmd.AddCommand(NewResumeCmd(cfg))
	cmd.AddCommand(NewPauseCmd(cfg))

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	flags.RegisterAuraCredentialFlag(cmd, cfg)

	return cmd
}
