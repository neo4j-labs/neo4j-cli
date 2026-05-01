// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package _import

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/import/job"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "import",
		Short: "Allows you to import your data into Aura instances and manage your import job",
	}

	cmd.AddCommand(job.NewCmd(cfg))

	return cmd
}
