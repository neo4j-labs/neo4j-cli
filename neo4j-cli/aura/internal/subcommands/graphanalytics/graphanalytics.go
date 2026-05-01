// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphanalytics

import (
	"fmt"

	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	sessions "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/graphanalytics/session"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "graph-analytics",
		Short: "Relates to Aura Graph Analytics",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg.Aura.BindBaseUrl(cmd.Flags().Lookup("base-url"))

			cfg.Aura.BindAuthUrl(cmd.Flags().Lookup("auth-url"))

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

	cmd.AddCommand(sessions.NewCmd(cfg))

	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")
	cmd.PersistentFlags().String("output", "", fmt.Sprintf("Format to print console output in, from a choice of [%s]", strings.Join(clicfg.ValidOutputValues[:], ", ")))

	return cmd
}
