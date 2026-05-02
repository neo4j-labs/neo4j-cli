// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/spf13/cobra"
)

var configPrintFields = []string{"key", "value"}

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage and view global configuration values",
	}

	cmd.AddCommand(NewGetCmd(cfg))
	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewSetCmd(cfg))
	if cfg.Aura.AuraBetaEnabled() {
		cmd.AddCommand(aura.NewAuraProjectCmd(cfg))
	}

	return cmd
}
