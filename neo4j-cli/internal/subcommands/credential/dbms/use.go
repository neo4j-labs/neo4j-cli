// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newUseCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Sets the default dbms credential to be used",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Dbms.SetDefault(args[0])
		},
	}
}
