// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:       "get <key>",
		Short:     "Displays the specified configuration value",
		ValidArgs: cfg.Aura.ValidConfigKeys,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			output.PrintBodyMap(cmd, cfg, cfg.Aura.GetPrintable(key), configPrintFields)
			return nil
		},
	}
}
