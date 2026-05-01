// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package _import

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/import/job"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "import",
		Short: "Allows you to import your data into Aura instances and manage your import job",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			outputValue := cmd.Flags().Lookup("output").Value.String()
			if outputValue != "" {
				validOutputValue := false
				for _, v := range clicfg.ValidOutputValues {
					if v == outputValue {
						validOutputValue = true
						break
					}
				}
				if !validOutputValue {
					return clierr.NewUsageError("invalid output value specified: %s", outputValue)
				}
			}

			cfg.Global.BindOutput(cmd.Flags().Lookup("output"))
			return nil
		},
	}

	cmd.AddCommand(job.NewCmd(cfg))
	cmd.PersistentFlags().String("output", "", fmt.Sprintf("Format to print console output in, from a choice of [%s]", strings.Join(clicfg.ValidOutputValues[:], ", ")))

	return cmd
}
