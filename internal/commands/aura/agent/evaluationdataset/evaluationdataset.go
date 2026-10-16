// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationdataset

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd returns the `aura agent evaluation-dataset` command group. The
// organization and project flags are registered on the agent parent, so the
// group adds none of its own.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "evaluation-dataset",
		Short: "Relates to Aura Agent evaluation datasets",
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newGetCmd(cfg))
	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newUpdateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))

	return cmd
}
