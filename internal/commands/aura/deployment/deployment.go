// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package deployment

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/deployment/database"
	"github.com/neo4j/cli/internal/commands/aura/deployment/server"
	"github.com/neo4j/cli/internal/commands/aura/deployment/token"
	auraflags "github.com/neo4j/cli/internal/commands/aura/flags"
	"github.com/neo4j/cli/internal/flags"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "deployment",
		Short: "Relates to Fleet Manager deployments",
		Long:  "Relates to Fleet Manager deployments: Neo4j DBMSs (self-managed or otherwise unmonitored) that report to Aura Fleet Manager. Scoped to a project.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))
			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

			return nil
		},
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newGetCmd(cfg))
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	cmd.AddCommand(token.NewCmd(cfg))
	cmd.AddCommand(database.NewCmd(cfg))
	cmd.AddCommand(server.NewCmd(cfg))

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	flags.RegisterAuraCredentialFlag(cmd, cfg)
	auraflags.RegisterOrgProjectFlags(cmd)

	return cmd
}
