// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/config/project"
	"github.com/spf13/cobra"
)

var configPrintFields = []string{"key", "value"}

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage and view configuration values",
	}

	cmd.AddCommand(NewGetCmd(cfg))
	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewSetCmd(cfg))
	if cfg.Aura.AuraBetaEnabled() {
		cmd.AddCommand(project.NewCmd(cfg))
	}

	return cmd
}

func getValidConfigKeys(cfg *clicfg.Config) []string {
	return append(append([]string{}, cfg.Aura.ValidConfigKeys[:]...), cfg.Global.ValidConfigKeys[:]...) // valid keys are both global and subcommand-specific keys
}
