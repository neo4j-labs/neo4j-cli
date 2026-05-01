// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewGetCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:       "get <key>",
		Short:     "Displays the specified global configuration value",
		ValidArgs: cfg.Global.ValidConfigKeys[:],
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := cfg.Global.Get(key)

			switch cfg.Global.Output() {
			case "json":
				printConfigKeyValueAsJSON(cmd, key, value)
			case "table":
				printConfigKeyValueAsTable(cmd, key, value)
			default:
				cmd.Println(value)
			}

			return nil
		},
	}
}
