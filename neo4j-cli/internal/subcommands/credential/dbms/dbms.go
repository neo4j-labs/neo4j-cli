// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dbms",
		Short: "Manage and view dbms credential values",
	}

	cmd.AddCommand(newAddCmd(cfg))
	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newRemoveCmd(cfg))
	cmd.AddCommand(newUseCmd(cfg))

	return cmd
}
