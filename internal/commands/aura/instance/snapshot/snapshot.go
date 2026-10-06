// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package snapshot

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "snapshot",
		Short: "Relates to an instance snapshots",
	}

	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewCreateCmd(cfg))
	cmd.AddCommand(NewGetCmd(cfg))

	return cmd
}
