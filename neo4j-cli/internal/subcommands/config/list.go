// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config

import (
	"github.com/neo4j/cli/common/clicfg"
	output "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

func NewListCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lists the current global configuration values",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			output.PrintBodyMap(cmd, cfg, cfg.Printable(), configPrintFields)
		},
	}
}
