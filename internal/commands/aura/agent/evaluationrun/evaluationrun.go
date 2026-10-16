// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "evaluation-run",
		Short: "Relates to Aura Agent evaluation runs",
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newGetCmd(cfg))
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))
	cmd.AddCommand(newCancelCmd(cfg))

	return cmd
}
