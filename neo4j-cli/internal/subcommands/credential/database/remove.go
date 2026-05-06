// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newRemoveCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Removes a database credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cfg.Credentials.Database.Remove(args[0])
		},
	}
}
