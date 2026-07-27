// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package virtualgraph

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	auraflags "github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "virtual-graph",
		Short: "Relates to Aura Virtual Graphs",
		Long: `Relates to Aura Virtual Graphs — Neo4j instances that query an external data source (for example Databricks, Snowflake or BigQuery) through a graph data model, without copying the data.

A virtual graph is built from a data source and a graph data model, both created in Data Importer, and is scoped to a project. Virtual Graphs must be enabled for your organization; if they are not, requests fail with a permission error.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))

			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

			return nil
		},
	}

	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newGetCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newUpdateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	cmd.AddCommand(newAllowedConfigsCmd(cfg))

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")

	flags.RegisterAuraCredentialFlag(cmd, cfg)
	auraflags.RegisterOrgProjectFlags(cmd)

	return cmd
}
