// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "database",
		Short: "Manage and view database credential values",
	}

	cmd.AddCommand(newAddCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newRemoveCmd(cfg))
	cmd.AddCommand(newUseCmd(cfg))

	return cmd
}
