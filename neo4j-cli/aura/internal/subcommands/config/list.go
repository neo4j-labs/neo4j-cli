// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	common_output "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists the current configuration of the Aura CLI subcommand",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			data := make(common_output.ConfigData, 0, len(cfg.Aura.ValidConfigKeys))
			for _, key := range cfg.Aura.ValidConfigKeys {
				data = append(data, common_output.ConfigEntry{Key: key, Value: cfg.Aura.Get(key)})
			}
			common_output.PrintBodyMap(cmd, cfg, data, []string{"key", "value"})
		},
	}
}
